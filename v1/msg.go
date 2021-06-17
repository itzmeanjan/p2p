package v1

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
)

type Msg struct {
	Id   uint32   `json:"id"`
	Hops []uint32 `json:"hops"`
}

func (m *Msg) read(rw *bufio.ReadWriter) (int, error) {
	buf := make([]byte, 4)
	if _, err := io.ReadFull(rw, buf); err != nil {
		return 0, err
	}

	size := binary.LittleEndian.Uint32(buf)

	buf = make([]byte, size)
	n, err := io.ReadFull(rw, buf)
	if err != nil {
		return 4, err
	}

	if err := json.Unmarshal(buf, &m); err != nil {
		return n + 4, err
	}

	if m == nil {
		m.Hops = make([]uint32, 0)
	}
	return n + 4, nil
}

func (m *Msg) write(rw *bufio.ReadWriter) (int, error) {
	marshalled, err := json.Marshal(&m)
	if err != nil {
		return 0, err
	}

	buf := make([]byte, 4+len(marshalled))
	binary.LittleEndian.PutUint32(buf[:4], uint32(len(marshalled)))
	n := copy(buf[4:], marshalled)
	if n != len(marshalled) {
		return 0, errors.New("full message not written")
	}

	n, err = rw.Write(buf)
	if err == nil {
		err = rw.Flush()
	}
	return n, err
}
