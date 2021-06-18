package v1

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
)

type Msg struct {
	Id   uint32   `json:"id"`
	Hops []uint32 `json:"hops"`
}

func (m *Msg) read(r io.Reader) (int, error) {
	var size uint32
	if err := binary.Read(r, binary.BigEndian, &size); err != nil {
		return 0, err
	}

	buf := make([]byte, size)
	n, err := r.Read(buf)
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

func (m *Msg) write(w io.Writer) (int, error) {
	marshalled, err := json.Marshal(&m)
	if err != nil {
		return 0, err
	}

	buf := make([]byte, 4+len(marshalled))
	binary.BigEndian.PutUint32(buf[:4], uint32(len(marshalled)))
	n := copy(buf[4:], marshalled)
	if n != len(marshalled) {
		return 0, errors.New("full message not written")
	}

	return w.Write(buf)
}
