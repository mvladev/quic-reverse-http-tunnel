package pipe

import (
	"io"
	"sync"
)

func Request(src, dst io.ReadWriteCloser) {
	var (
		wg sync.WaitGroup
		o  sync.Once
	)

	close := func() {
		src.Close()
		dst.Close()
	}

	wg.Add(2)

	go func() {
		_, _ = io.Copy(src, dst)
		o.Do(close)
		wg.Done()
	}()

	go func() {
		_, _ = io.Copy(dst, src)
		o.Do(close)
		wg.Done()
	}()

	wg.Wait()

}
