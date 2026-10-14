package client

import (
	"encoding/binary"
	"os"
)

type WAV struct {
	file  *os.File
	bytes uint32
}

func NewWAV(path string) (*WAV, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	w := &WAV{file: f}
	if err = w.header(); err != nil {
		f.Close()
		return nil, err
	}
	return w, nil
}
func (w *WAV) header() error {
	h := make([]byte, 44)
	copy(h, "RIFF")
	binary.LittleEndian.PutUint32(h[4:], w.bytes+36)
	copy(h[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(h[16:], 16)
	binary.LittleEndian.PutUint16(h[20:], 1)
	binary.LittleEndian.PutUint16(h[22:], 1)
	binary.LittleEndian.PutUint32(h[24:], 16000)
	binary.LittleEndian.PutUint32(h[28:], 32000)
	binary.LittleEndian.PutUint16(h[32:], 2)
	binary.LittleEndian.PutUint16(h[34:], 16)
	copy(h[36:], "data")
	binary.LittleEndian.PutUint32(h[40:], w.bytes)
	_, err := w.file.WriteAt(h, 0)
	return err
}
func (w *WAV) Write(data []byte) error {
	n, err := w.file.WriteAt(data, int64(44+w.bytes))
	w.bytes += uint32(n)
	if err != nil {
		return err
	}
	if err = w.header(); err != nil {
		return err
	}
	return nil
}
func (w *WAV) Close() error {
	err := w.header()
	if err == nil {
		err = w.file.Sync()
	}
	closeErr := w.file.Close()
	if err != nil {
		return err
	}
	return closeErr
}
