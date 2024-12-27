package codec_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"github.com/brickingsoft/rio/codec"
	"github.com/brickingsoft/rxp"
	"github.com/brickingsoft/rxp/async"
	"io"
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
	codec.LengthFieldDecode(ctx, r, 8).OnComplete(func(ctx context.Context, msg []byte, err error) {
		if err != nil {
			if async.IsEOF(err) {
				wg.Done()
				return
			}
			if !errors.Is(err, io.EOF) {
				t.Error(err)
			}
			return
		}
		t.Log(bytes.Equal(msg, b), string(msg))
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
	codec.LengthFieldEncode(ctx, w, b, 8).OnComplete(func(ctx context.Context, result int, err error) {
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
