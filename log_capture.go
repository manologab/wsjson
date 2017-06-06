package wsjson

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
)

type logCapture struct {
	outBck   io.Writer
	buffer   []string
	bufMutex sync.Mutex
	writer   *io.PipeWriter
	reader   *io.PipeReader
	done     chan bool
}

func startLogCapture() *logCapture {
	rd, wr := io.Pipe()
	done := make(chan bool, 1)
	lc := &logCapture{
		outBck: os.Stderr,
		buffer: make([]string, 0, 10),
		writer: wr,
		reader: rd,
		done:   done,
	}

	log.SetOutput(wr)

	go func() {
		scan := bufio.NewScanner(rd)

		for i := 0; scan.Scan(); i++ {
			lc.bufMutex.Lock()
			lc.buffer = append(lc.buffer, scan.Text())
			lc.bufMutex.Unlock()
		}
		if err := scan.Err(); err != nil {
			panic(fmt.Sprintf("Error reading logs: %v", err))
		}
		lc.done <- true

	}()

	return lc
}

func (lc *logCapture) contains(term string) bool {
	lc.bufMutex.Lock()
	defer lc.bufMutex.Unlock()
	for _, entry := range lc.buffer {
		if strings.Contains(entry, term) {
			return true
		}
	}
	return false
}

func (lc *logCapture) stop() {
	lc.writer.Close()
	log.SetOutput(lc.outBck)
	<-lc.done
}
