package v1

import (
	"encoding/binary"
	"encoding/json"
	"io"
)

type Msg struct {
	Id   uint8   `json:"id"`
	Hops []uint8 `json:"hops"`
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

	return n + 4, json.Unmarshal(buf, &m)
}

func (m *Msg) write(w io.Writer) (int, error) {
	marshalled, err := json.Marshal(&m)
	if err != nil {
		return 0, err
	}

	if err := binary.Write(w, binary.BigEndian, uint32(len(marshalled))); err != nil {
		return 0, err
	}

	n, err := w.Write(marshalled)
	if err != nil {
		return 4, err
	}

	return n + 4, nil
}
