package codec_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"github.com/brickingsoft/rio/codec"
	"github.com/brickingsoft/rxp"
	"sync"
	"testing"
)

func TestLengthFieldDecode(t *testing.T) {
	ctx := context.Background()
	exec := rxp.New()
	defer exec.Close()
	ctx = rxp.With(ctx, exec)

	b := []byte("hello world")
	p := make([]byte, 8+len(b))
	binary.BigEndian.PutUint64(p, uint64(len(b)))
	copy(p[8:], b)
	r := newFakeReader(ctx, p)
	wg := new(sync.WaitGroup)
	wg.Add(1)
	codec.LengthFieldDecode(ctx, r).OnComplete(func(ctx context.Context, msg codec.LengthFieldMessage, err error) {
		defer wg.Done()
		if err != nil {
			t.Error(err)
			return
		}
		t.Log(bytes.Equal(msg.Bytes, b), string(msg.Bytes))
		return
	})
	wg.Wait()
}

func TestLengthFieldEncode(t *testing.T) {
	ctx := context.Background()
	b := []byte("hello world")
	w := newFakeWriter(ctx)
	wg := new(sync.WaitGroup)
	wg.Add(1)
	codec.LengthFieldEncode(ctx, w, b).OnComplete(func(ctx context.Context, result int, err error) {
		defer wg.Done()
		if err != nil {
			t.Error(err)
			return
		}
		wn := result
		p := make([]byte, 8+len(b))
		binary.BigEndian.PutUint64(p, uint64(len(b)))
		copy(p[8:], b)
		t.Log(wn, len(p), w.Equals(p))
	})
	wg.Wait()
}
