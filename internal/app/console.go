package app

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

type Console interface {
	Print(msg any)
	Printf(format string, args ...any)
	Println(msg ...any)
	Errorf(format string, args ...any)
	ReadLine() (string, error)
}

type StdIO struct {
	in     *bufio.Reader
	out    io.Writer
	errOut io.Writer
}

func NewStdIO() *StdIO {
	return &StdIO{
		in:     bufio.NewReader(os.Stdin),
		out:    os.Stdout,
		errOut: os.Stderr,
	}
}

func (c *StdIO) Print(msg any) {
	_, _ = fmt.Fprint(c.out, msg)
}

func (c *StdIO) Printf(format string, args ...any) {
	_, _ = fmt.Fprintf(c.out, format, args...)
}

func (c *StdIO) Println(msg ...any) {
	_, _ = fmt.Fprintln(c.out, msg...)
}

func (c *StdIO) Errorf(format string, args ...any) {
	_, _ = fmt.Fprintf(c.errOut, format, args...)
}

func (c *StdIO) ReadLine() (string, error) {
	line, err := c.in.ReadString('\n')
	if err != nil {
		if len(line) == 0 {
			return "", err
		}
		trimmed := strings.TrimSpace(strings.TrimSuffix(line, "\r\n"))
		if trimmed == "" {
			return "", nil
		}
		return trimmed, nil
	}

	return strings.TrimSpace(strings.TrimSuffix(line, "\r\n")), nil
}
