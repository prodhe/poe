package gapbuffer_test

import (
	"io"
	"testing"

	"github.com/prodhe/poe/gapbuffer"
)

const lipsum = `Fusce vitae molestie tortor. Fusce congue ornare risus vitae dignissim. Praesent volutpat erat sit amet posuere varius. Fusce id fermentum risus. In ac eros varius, fringilla erat ac, cursus odio. Nunc consectetur vitae dolor non cursus. Sed eleifend imperdiet sem sit amet rutrum. Nulla pretium et ante eu lobortis. Suspendisse porta sodales fermentum.

Proin dignissim lorem sed leo aliquam rutrum. Donec vel lorem vitae dui mollis lobortis. Nam ac ornare tellus, ac venenatis nulla. Curabitur sagittis at nulla id blandit. Curabitur porta sit amet orci sed aliquam. Duis fermentum tincidunt rutrum. Morbi varius blandit velit in interdum. Cras congue pretium nisl nec interdum.

Sed rutrum risus sed mauris cursus, nec viverra dolor blandit. Vestibulum commodo malesuada felis vitae varius. Nulla euismod id felis eu pulvinar. Fusce nisl mauris, pretium quis dignissim sit amet, consequat sed tortor. Vestibulum sollicitudin mi risus, vitae porta dolor semper vitae. Vestibulum tristique libero ac mauris tristique, at blandit ipsum facilisis. In mollis diam a dapibus fermentum. Phasellus laoreet diam id pretium faucibus. Orci varius natoque penatibus et magnis dis parturient montes, nascetur ridiculus mus. Suspendisse ornare velit eget lorem tincidunt vehicula. Duis purus augue, finibus ut dolor nec, tempor vulputate ipsum. Ut pellentesque id sem quis luctus. Integer tristique facilisis eleifend. Sed at egestas risus.`

func sliceEqual(a []byte, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestReadAt(t *testing.T) {
	gb := gapbuffer.Buffer{}
	gb.Write([]byte(lipsum))

	var tt = []struct {
		name      string
		offset    int
		len       int
		want      int
		wantbytes []byte
		wanterr   error
	}{
		{"first", 0, 11, 11, []byte("Fusce vitae"), nil},
		{"middle", 21, 20, 20, []byte("tortor. Fusce congue"), nil},
		{"last", gb.Len() - 1, 1, 1, []byte("."), nil},
		{"out of range below", -1, 0, 0, nil, gapbuffer.ErrOutOfRange},
		{"out of range above", 9999, 0, 0, nil, io.EOF},
	}

	for _, tc := range tt {
		b := make([]byte, tc.len)
		gb.Seek(0)
		n, err := gb.ReadAt(b, tc.offset)
		if n != tc.len || err != tc.wanterr || !sliceEqual(b, tc.wantbytes) {
			t.Errorf("seek 0: %s: expected %q %x (%d) (err: %v) (len: %d), got %q %x (%d) (err: %v) (len: %d)", tc.name, tc.wantbytes, tc.wantbytes, tc.want, tc.wanterr, tc.len, b, b, n, err, len(b))
		}
	}
}

func TestSeekAndPos(t *testing.T) {
	gb := gapbuffer.Buffer{}
	gb.Write([]byte(lipsum))

	var tt = []struct {
		name   string
		offset int
		want   int
	}{
		{"beginning", 0, 0},
		{"middle", 25, 25},
		{"end", len(lipsum), len(lipsum)},
	}

	for _, tc := range tt {
		gb.Seek(tc.offset)
		pos := gb.Pos()
		if pos != tc.want {
			t.Errorf("%s: set %d, expected %d, got %d", tc.name, tc.offset, tc.want, pos)
		}
	}
}

func TestWriteSingle(t *testing.T) {
	var tt = []struct {
		name       string
		input      []byte
		wantret    int
		wantreterr error
	}{
		{"nil", nil, 0, nil},
		{"empty", []byte{}, 0, nil},
		{"null byte", []byte{'0'}, 1, nil},
		{"string", []byte("gopher"), 6, nil},
		{"control characters", []byte{'\u0011', '\u0012', '\u0013'}, 3, nil},
	}

	for _, tc := range tt {
		b := gapbuffer.Buffer{}
		n, err := b.Write(tc.input)
		if n != tc.wantret {
			t.Errorf("%s: expected %d bytes, got %d", tc.name, tc.wantret, n)
		}
		if err != tc.wantreterr {
			t.Errorf("%s: expected error %v, got %v", tc.name, tc.wantreterr, err)
		}
	}
}

func TestWriteMulti(t *testing.T) {
	var tt = []struct {
		name   string
		input1 []byte
		offset int
		input2 []byte
		want   []byte
	}{
		{"beginning", []byte("bar"), 0, []byte("foo"), []byte("foobar")},
		{"end", []byte("foo"), 3, []byte("bar"), []byte("foobar")},
		{"middle", []byte("hello gopher"), 5, []byte(" you"), []byte("hello you gopher")},
		{"expand buffer", []byte("abcdefghijklmnopqrstuvwxyz"), 8, []byte("0123456789"), []byte("abcdefgh0123456789ijklmnopqrstuvwxyz")},
	}

	for _, tc := range tt {
		b := gapbuffer.Buffer{}
		b.Write(tc.input1)
		b.Seek(tc.offset)
		b.Write(tc.input2)
		data := b.Bytes()
		if !sliceEqual(data, tc.want) {
			t.Errorf("%s: wrote %q, then %q at offset %d, got %q (len: %d, cap: %d)", tc.name, tc.input1, tc.input2, tc.offset, data, b.Len(), b.Cap())
		}
	}
}

func TestByteAt(t *testing.T) {
	var tt = []struct {
		name    string
		input   []byte
		pos     int
		want    byte
		wanterr error
	}{
		{"first", []byte("abcdefghij"), 0, 'a', nil},
		{"middle", []byte("abcdefghij"), 4, 'e', nil},
		{"last", []byte("abcdefghij"), 9, 'j', nil},
		{"lipsum", []byte(lipsum), 40, 'e', nil},
		{"out of range below", []byte("abcdefghij"), -1, 0, gapbuffer.ErrOutOfRange},
		{"out of range above", []byte("abcdefghij"), 999, 0, gapbuffer.ErrOutOfRange},
	}

	for _, tc := range tt {
		b := gapbuffer.Buffer{}
		b.Write(tc.input)
		b.Seek(0)
		c, err := b.ByteAt(tc.pos)
		if c != tc.want {
			t.Errorf("Seek(0): %s: expected %c (%x) and %v, got %c (%x) and %v", tc.name, tc.want, tc.want, tc.wanterr, c, c, err)
		}
	}

	for _, tc := range tt {
		b := gapbuffer.Buffer{}
		b.Write(tc.input)
		b.Seek(2)
		c, err := b.ByteAt(tc.pos)
		if c != tc.want {
			t.Errorf("Seek(2): %s: expected %c (%x) and %v, got %c (%x) and %v", tc.name, tc.want, tc.want, tc.wanterr, c, c, err)
		}
	}

	for _, tc := range tt {
		b := gapbuffer.Buffer{}
		b.Write(tc.input)
		b.Seek(b.Len())
		c, err := b.ByteAt(tc.pos)
		if c != tc.want {
			t.Errorf("Seek(len): %s: expected %c (%x) and %v, got %c (%x) and %v", tc.name, tc.want, tc.want, tc.wanterr, c, c, err)
		}
	}
}
