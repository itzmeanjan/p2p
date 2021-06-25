package peer

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
)

type Message struct {
	Author int64   `json:"author"`
	Kind   string  `json:"kind"`
	Peer   int64   `json:"peer,omitempty"`
	Hops   []int64 `json:"hops,omitempty"`
}

func (m *Message) Write(rw *bufio.ReadWriter) (int, error) {
	b, err := json.Marshal(m)
	if err != nil {
		return 0, err
	}

	chunk := make([]byte, 4+len(b))
	binary.BigEndian.PutUint32(chunk[:4], uint32(len(b)))
	n := copy(chunk[4:], b)
	if n != len(b) {
		return 0, errors.New("partial copy")
	}

	n, err = rw.Write(chunk)
	if err != nil {
		return n, err
	}
	if n != len(chunk) {
		return n, errors.New("partial write")
	}

	return n, rw.Flush()
}

func (m *Message) Read(rw *bufio.ReadWriter) (int, error) {
	var total int
	buf := make([]byte, 4)
	n, err := io.ReadFull(rw.Reader, buf)
	if err != nil {
		return n, err
	}

	total += n
	size := binary.BigEndian.Uint32(buf)
	buf = make([]byte, size)
	n, err = io.ReadFull(rw.Reader, buf)
	if err != nil {
		return total + n, err
	}

	total += n
	return total, json.Unmarshal(buf, m)
}
