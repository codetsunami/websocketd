// Copyright 2013 Joe Walnes and the websocketd team.
// All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libwebsocketd

import (
    "bufio"
    "io"
    "syscall"
    "time"
    "encoding/binary"
    "fmt"
)

type ProcessEndpoint struct {
    process   *LaunchedProcess
    closetime time.Duration
    output    chan []byte
    log       *LogScope
    bin       bool
    sh        bool  // use 8 byte size header
    mf        int   // max frame size if sizeheader is enabled
}

func NewProcessEndpoint(process *LaunchedProcess, bin bool, sh bool, maxframe uint, log *LogScope) *ProcessEndpoint {
    return &ProcessEndpoint{
        process: process,
        output:  make(chan []byte),
        log:     log,
        bin:     bin,
        sh:      sh,
        mf:      int(maxframe),
    }
}

func (pe *ProcessEndpoint) Terminate() {
    terminated := make(chan struct{})
    go func() { pe.process.cmd.Wait(); terminated <- struct{}{} }()

    // for some processes this is enough to finish them...
    pe.process.stdin.Close()

    // a bit verbose to create good debugging trail
    select {
    case <-terminated:
        pe.log.Debug("process", "Process %v terminated after stdin was closed", pe.process.cmd.Process.Pid)
        return // means process finished
    case <-time.After(100*time.Millisecond + pe.closetime):
    }

    err := pe.process.cmd.Process.Signal(syscall.SIGINT)
    if err != nil {
        // process is done without this, great!
        pe.log.Error("process", "SIGINT unsuccessful to %v: %s", pe.process.cmd.Process.Pid, err)
    }

    select {
    case <-terminated:
        pe.log.Debug("process", "Process %v terminated after SIGINT", pe.process.cmd.Process.Pid)
        return // means process finished
    case <-time.After(250*time.Millisecond + pe.closetime):
    }

    err = pe.process.cmd.Process.Signal(syscall.SIGTERM)
    if err != nil {
        // process is done without this, great!
        pe.log.Error("process", "SIGTERM unsuccessful to %v: %s", pe.process.cmd.Process.Pid, err)
    }

    select {
    case <-terminated:
        pe.log.Debug("process", "Process %v terminated after SIGTERM", pe.process.cmd.Process.Pid)
        return // means process finished
    case <-time.After(500*time.Millisecond + pe.closetime):
    }

    err = pe.process.cmd.Process.Kill()
    if err != nil {
        pe.log.Error("process", "SIGKILL unsuccessful to %v: %s", pe.process.cmd.Process.Pid, err)
        return
    }

    select {
    case <-terminated:
        pe.log.Debug("process", "Process %v terminated after SIGKILL", pe.process.cmd.Process.Pid)
        return // means process finished
    case <-time.After(1000 * time.Millisecond):
    }

    pe.log.Error("process", "SIGKILL did not terminate %v!", pe.process.cmd.Process.Pid)
}

func (pe *ProcessEndpoint) Output() chan []byte {
    return pe.output
}

func (pe *ProcessEndpoint) Send(msg []byte) bool {
    pe.process.stdin.Write(msg)
    return true
}

func (pe *ProcessEndpoint) StartReading() {
    go pe.log_stderr()
    if pe.sh {
        go pe.process_shout()
    } else if pe.bin {
        go pe.process_binout()
    } else {
        go pe.process_txtout()
    }
}


func (pe *ProcessEndpoint) process_shout() {
    headerbuf := make([]byte, 4)
    buf := make([]byte, pe.mf)
    for {

        for discard_first := 0; discard_first < 2; discard_first++ { // first 4 bytes are reserved, second 4 are size
            for upto := 0; upto < 4;  {
                n, err := pe.process.stdout.Read(headerbuf[upto:])
                if err != nil {
                    if err != io.EOF {
                        pe.log.Error("process", "Unexpected error while reading STDOUT from process: %s", err)
                    } else {
                        pe.log.Debug("process", "Process STDOUT closed")
                    }
                    close(pe.output)
                    return
                }
                upto += n
            }
        }

        framesize := int(binary.BigEndian.Uint32(headerbuf))

        fmt.Println("[DEBUG] Received frame from process, size:", framesize) 

        if framesize >= pe.mf {
            pe.log.Error("process", "Oversized frame, terminating") //TODO: consider IP banning (after repeated attempts) the user sending oversize frames?
            break
        }

        // execution to here means we can read the whole pending frame
        for upto := 0; upto < framesize;  {
            n, err := pe.process.stdout.Read(buf[upto:])
            if err != nil {
                if err != io.EOF {
                    pe.log.Error("process", "Unexpected error while reading STDOUT from process: %s", err)
                } else {
                    pe.log.Debug("process", "Process STDOUT closed")
                }
                break
            }
            upto += n
        }

        pe.output <- append(make([]byte, 0, framesize), buf[:framesize]...) // cloned buffer
    }
    close(pe.output)
}

func (pe *ProcessEndpoint) process_txtout() {
    bufin := bufio.NewReader(pe.process.stdout)
    for {
        buf, err := bufin.ReadBytes('\n')
        if err != nil {
            if err != io.EOF {
                pe.log.Error("process", "Unexpected error while reading STDOUT from process: %s", err)
            } else {
                pe.log.Debug("process", "Process STDOUT closed")
            }
            break
        }
        pe.output <- trimEOL(buf)
    }
    close(pe.output)
}

func (pe *ProcessEndpoint) process_binout() {
    buf := make([]byte, 10*1024*1024)
    for {
        n, err := pe.process.stdout.Read(buf)
        if err != nil {
            if err != io.EOF {
                pe.log.Error("process", "Unexpected error while reading STDOUT from process: %s", err)
            } else {
                pe.log.Debug("process", "Process STDOUT closed")
            }
            break
        }
        pe.output <- append(make([]byte, 0, n), buf[:n]...) // cloned buffer
    }
    close(pe.output)
}

func (pe *ProcessEndpoint) log_stderr() {
    bufstderr := bufio.NewReader(pe.process.stderr)
    for {
        buf, err := bufstderr.ReadSlice('\n')
        if err != nil {
            if err != io.EOF {
                pe.log.Error("process", "Unexpected error while reading STDERR from process: %s", err)
            } else {
                pe.log.Debug("process", "Process STDERR closed")
            }
            break
        }
        pe.log.Error("stderr", "%s", string(trimEOL(buf)))
    }
}

// trimEOL cuts unixy style \n and windowsy style \r\n suffix from the string
func trimEOL(b []byte) []byte {
    lns := len(b)
    if lns > 0 && b[lns-1] == '\n' {
        lns--
        if lns > 0 && b[lns-1] == '\r' {
            lns--
        }
    }
    return b[:lns]
}
