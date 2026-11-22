package proc

// 상한 걸린 실행(run.go)의 출력 관(재감사 2바퀴 P4 — merger R-212·R-213·R-219·R-220): 파일이 아닌 c.Stdout·c.Stderr마다 os.Pipe 하나와
// 옮기는 고루틴 하나. 결과 수집(관을 끝까지 읽음)과 부른 쪽 쓰개의 수명을 가른다 — 쓰개가 막혀도 부른 쪽은 마감 + 회수 유예 안에
// 돌아오고, 돌아온 뒤에는 쓰개에 새로 쓰지 않는다. 쓰개 오류는 결과 오류다. 읽는 끝 fd는 어느 길에서나 돌아오기 전에 닫는다.

import (
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
)

type outPipe struct {
	rs, ws []*os.File
	wg     sync.WaitGroup
	done   chan struct{} // 옮기기가 다 끝나면(모든 쓰는 쪽이 닫혀 EOF, 또는 abandon) 닫힌다
	wmu    sync.Mutex    // 쓰개에 쓰는 동안 쥔다 — abandon이 이것을 잡으면 그 뒤 새 쓰기는 없다
	closed atomic.Bool   // abandon 뒤 — 쓰개에 새로 쓰지 않는다
	errMu  sync.Mutex
	werr   error // 첫 쓰개 오류
}

// takeOutput: c의 파일 아닌 출력을 관으로 바꾼다(같은 Writer면 관 하나 — exec와 같음)
func takeOutput(c *exec.Cmd) (*outPipe, error) {
	o := &outPipe{done: make(chan struct{})}
	same := sameWriter(c.Stdout, c.Stderr)
	var err error
	c.Stdout, err = o.wrap(c.Stdout)
	if err == nil {
		if same {
			c.Stderr = c.Stdout
		} else {
			c.Stderr, err = o.wrap(c.Stderr)
		}
	}
	go func() { o.wg.Wait(); close(o.done) }()
	if err != nil {
		o.abandon(0)
		return nil, err
	}
	return o, nil
}

func sameWriter(a, b io.Writer) (same bool) {
	defer func() {
		if recover() != nil {
			same = false // 견줄 수 없는 꼴
		}
	}()
	return a != nil && a == b
}

func (o *outPipe) wrap(w io.Writer) (io.Writer, error) {
	if w == nil {
		return nil, nil
	}
	if f, ok := w.(*os.File); ok {
		return f, nil
	}
	r, pw, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	o.rs, o.ws = append(o.rs, r), append(o.ws, pw)
	o.wg.Add(1)
	go o.copy(w, r)
	return pw, nil
}

// copy: r를 끝까지 읽어 w에 — 쓰개 오류가 나면 기억하고 나머지는 버리며 끝까지 읽는다(자식이 꽉 찬 관에 막히지 않게).
// abandon 뒤에는 쓰지 않는다
func (o *outPipe) copy(w io.Writer, r *os.File) {
	defer o.wg.Done()
	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			o.wmu.Lock()
			if !o.closed.Load() && o.err() == nil {
				if m, werr := w.Write(buf[:n]); werr != nil || m != n {
					if werr == nil {
						werr = io.ErrShortWrite
					}
					o.errMu.Lock()
					o.werr = werr
					o.errMu.Unlock()
				}
			}
			o.wmu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

func (o *outPipe) err() error {
	o.errMu.Lock()
	defer o.errMu.Unlock()
	return o.werr
}

// started: 자식이 떴다 — 부모 쪽 쓰는 끝을 닫는다(남은 쓰는 끝은 자식·손자의 것)
func (o *outPipe) started() {
	for _, w := range o.ws {
		w.Close()
	}
}

// drainedBy: 출력이 deadline 전에 다 옮겨졌는가(cancel이 닫혀도 거짓)
func (o *outPipe) drainedBy(deadline time.Time, cancel <-chan struct{}) bool {
	select {
	case <-o.done:
		return true
	default:
	}
	t := time.NewTimer(time.Until(deadline))
	defer t.Stop()
	select {
	case <-o.done:
		return true
	case <-t.C:
	case <-cancel:
	}
	select { // 마감과 같은 때 끝났으면 받아 준다
	case <-o.done:
		return true
	default:
		return false
	}
}

// close: 다 옮겨진 뒤 읽는 끝을 닫는다
func (o *outPipe) close() {
	for _, r := range o.rs {
		r.Close()
	}
}

// abandon: 관을 닫고(새로 쓰지 않게 표시한 뒤) 옮기는 고루틴이 끝나기를 wait까지 기다린다 — 쓰개가 막혀 있으면 그 한 번의 쓰기는
// 놓아 두고 돌아온다(돌아온 뒤 새 쓰기는 없다)
func (o *outPipe) abandon(wait time.Duration) {
	o.closed.Store(true)
	for _, f := range append(append([]*os.File{}, o.ws...), o.rs...) {
		f.Close()
	}
	// 쓰는 중인 고루틴이 없음을 wmu로 확인한다 — 잡으면 그 뒤 쓰기는 closed를 보고 하지 않는다. 못 잡으면(쓰개가 막힘) 그 한 번은
	// 돌아오기 전에 시작된 쓰기다
	for end := time.Now().Add(wait); ; time.Sleep(5 * time.Millisecond) {
		if o.wmu.TryLock() {
			o.wmu.Unlock()
			break
		}
		if !time.Now().Before(end) {
			break
		}
	}
	select {
	case <-o.done:
	case <-time.After(wait):
	}
}
