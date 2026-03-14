package terminal

import (
	"bytes"
	"testing"
)

func TestBufferWrite(t *testing.T) {
	b := NewBuffer()
	b.Write([]byte("hello"))
	if got := string(b.Bytes()); got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestBufferAppend(t *testing.T) {
	b := NewBuffer()
	b.Write([]byte("hello "))
	b.Write([]byte("world"))
	if got := string(b.Bytes()); got != "hello world" {
		t.Errorf("got %q, want %q", got, "hello world")
	}
}

func TestBufferOverflow(t *testing.T) {
	b := NewBuffer()
	// Write more than maxBufferSize
	big := bytes.Repeat([]byte("A"), maxBufferSize+1000)
	b.Write(big)
	if b.Len() > maxBufferSize {
		t.Errorf("buffer size %d exceeds max %d", b.Len(), maxBufferSize)
	}
}

func TestBufferUTF8Safety(t *testing.T) {
	b := NewBuffer()
	// Fill buffer almost to max
	filler := bytes.Repeat([]byte("A"), maxBufferSize-2)
	b.Write(filler)

	// Write a multi-byte UTF-8 character (€ = 3 bytes: 0xE2 0x82 0xAC)
	// This should push buffer over limit, forcing a trim
	b.Write([]byte("€€€"))

	data := b.Bytes()
	if len(data) == 0 {
		t.Fatal("buffer should not be empty")
	}
	// First byte should not be a continuation byte (0x80-0xBF)
	if data[0]&0xC0 == 0x80 {
		t.Errorf("buffer starts with UTF-8 continuation byte: 0x%02x", data[0])
	}
}

func TestBufferBytesIsCopy(t *testing.T) {
	b := NewBuffer()
	b.Write([]byte("original"))
	got := b.Bytes()
	got[0] = 'X'
	if string(b.Bytes()) != "original" {
		t.Error("Bytes() should return a copy, not a reference")
	}
}

func TestBufferEmpty(t *testing.T) {
	b := NewBuffer()
	if b.Len() != 0 {
		t.Errorf("new buffer should be empty, got %d", b.Len())
	}
	if len(b.Bytes()) != 0 {
		t.Error("new buffer Bytes() should be empty")
	}
}
