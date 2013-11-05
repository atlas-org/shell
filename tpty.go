//+build ignore

package main

import (
	"bytes"
	"fmt"
	//"io"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/gonuts/pty"
)

const delay = 50 * time.Microsecond

type Request struct {
	cmd []byte
	err error
}

type Response struct {
	cmd []byte
	out []byte
	err error
}

func main() {

	cmd := exec.Command("/bin/sh")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setctty: true, Setsid: true}

	fd_pty, fd_tty, err := pty.Open()
	if err != nil {
		panic(err)
	}

	cmd.Stdin = fd_tty
	cmd.Stdout = fd_tty
	cmd.Stderr = fd_tty

	f, err := os.Create("log.txt")
	if err != nil {
		panic(err)
	}
	defer f.Close()
	//go io.Copy(f, fd_pty)

	err = cmd.Start()
	if err != nil {
		panic(err)
	}

	initch := make(chan Request)
	initdone := make(chan struct{})
	ich := make(chan Request)
	och := make(chan Response)
	done := make(chan error)

	pattern := func(buf *[]byte, pat []byte) ([]byte, []byte) {
		head := make([]byte, 0, 512)
		tail := make([]byte, 0, 512)
		fmt.Fprintf(os.Stderr, "...read resp... [pat=%q] (buf=%q)\n", string(pat), string(*buf))
		if idx := bytes.Index(*buf, pat); idx != -1 {
			head = (*buf)[:idx]
			tail = (*buf)[idx+len(pat):]
			return head, tail
		}
		for {
			fmt.Fprintf(os.Stderr, "...acc...\n")
			acc := make([]byte, 1024)
			n, err := fd_pty.Read(acc)
			if err != nil {
				continue
			}
			acc = acc[:n]
			*buf = append(*buf, acc...)
			fmt.Fprintf(os.Stderr, "...acc... (%q)\n", string(*buf))
			if idx := bytes.Index(*buf, pat); idx != -1 {
				head = (*buf)[:idx]
				tail = (*buf)[idx+len(pat):]
				break
			}
			fmt.Fprintf(os.Stderr, "+++ %q\n", string(*buf))
		}
		return head, tail
	}

	go func() {
		err := cmd.Wait()
		done <- err
	}()

	go func() {
		stream := make([]byte, 0, 1024)
		for {
			select {
			case req := <-initch:
				fmt.Fprintf(os.Stderr, "...write ireq...\n")
				_, err := fd_pty.Write(req.cmd)
				if err != nil {
					continue
				}
				_, buf := pattern(&stream, req.cmd)
				fmt.Fprintf(os.Stderr, "...got iresp... (%q)\n", string(buf))
				_, stream = pattern(&stream, []byte("term-is:vt100\r\n"))
				fmt.Fprintf(os.Stderr, "---got iresp... (%q)\n", string(buf))
				initdone <- struct{}{}

			case req := <-ich:
				go func() {
					fmt.Fprintf(os.Stderr, "...write req...\n")
					_, err := fd_pty.Write(req.cmd)
					if err != nil {
						och <- Response{cmd: req.cmd, err: err}
						return
					}
					_, stream = pattern(&stream, []byte("\r\ngosh> "))
					_, cmd_tail := pattern(&stream, req.cmd)
					stream = append([]byte{}, cmd_tail...)
					out_head, stream := pattern(&stream, []byte("\r\ngosh> "))
					// buf := make([]byte, 1024)
					// fmt.Fprintf(os.Stderr, "...read resp...\n")
					// n, err := fd_pty.Read(buf)
					// if err != nil {
					// 	och <- Response{cmd: req.cmd, err: err}
					// 	continue
					// }
					// buf = buf[:n]
					buf := append(cmd_tail, out_head...)
					fmt.Fprintf(os.Stderr, "...send resp... (tail=%q)\n", string(stream))
					och <- Response{cmd: req.cmd, out: buf, err: err}
					fmt.Fprintf(os.Stderr, "...send resp... (%q) [done]\n", string(buf))
				}()
			case err := <-done:
				close(done)
				// //close(ich)
				// //close(och)
				// err = cmd.Wait()
				// if err != nil {
				// 	panic(err)
				// }
				if err != nil {
					panic(err)
				}
				return
			}
		}
	}()

	isend := func(str string) error {
		fmt.Fprintf(os.Stderr, ">>> init-sending [%q]...\n", string(str))
		initch <- Request{cmd: []byte(str + "\r\n"), err: nil}
		return nil
	}

	send := func(str string) error {
		fmt.Fprintf(os.Stderr, ">>> sending [%q]...\n", string(str))
		ich <- Request{cmd: []byte(str + "\r\n"), err: nil}
		return nil
	}

	recv := func() ([]byte, error) {
		fmt.Fprintf(os.Stderr, "<<< recv...\n")
		r := <-och
		return r.out, r.err
	}

	err = isend(`export PS1="gosh> ";export TERM=vt100;echo "term-is:$TERM"`)
	if err != nil {
		panic(err)
	}
	<-initdone

	err = send("/bin/ls /tmp/binet/tototata")
	if err != nil {
		panic(err)
	}
	out, err := recv()
	if err != nil {
		panic(err)
	} else {
		fmt.Printf("### %q\n", string(out))
	}

	err = send(`echo "term=$TERM"`)
	if err != nil {
		panic(err)
	}
	out, err = recv()
	if err != nil {
		panic(err)
	} else {
		fmt.Printf("### %q\n", string(out))
	}

	err = send("exit")
	if err != nil {
		panic(err)
	}
	out, err = recv()
	if err != nil {
		panic(err)
	} else {
		fmt.Printf("### %q\n", string(out))
	}

	//io.Copy(f, fd_pty)

	//done <- struct{}{}
}

// EOF
