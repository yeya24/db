// Copyright 2021 The Conprof Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package chunkenc

import (
	"bytes"
	"encoding/binary"
	"io"

	"github.com/klauspost/compress/zstd"
)

// valueChunk needs everything the ByteChunk does except timestamps.
// The ValueIterator should just return []byte like the timestampChunk just returns timestamps.
// The appender should just add the []byte as they are passed, no compression etc. (yet).

type valueChunk struct {
	compressed []byte // only read once into b to decompress
	b          []byte // never read, only written to be able to quickly compress
	num        uint16
}

func newValueChunk() *valueChunk {
	return &valueChunk{b: make([]byte, 0, 5000)}
}

func (c *valueChunk) Bytes() []byte {
	buf := &bytes.Buffer{}
	//w := snappy.NewBufferedWriter(buf)
	w, _ := zstd.NewWriter(buf, // TODO handle error
		zstd.WithEncoderLevel(zstd.SpeedFastest),
	)
	_, _ = io.Copy(w, bytes.NewBuffer(c.b)) // TODO handle error
	_ = w.Close()                           // TODO handle error
	return buf.Bytes()
}

func (c *valueChunk) Encoding() Encoding {
	return EncValues
}

func (c *valueChunk) NumSamples() int {
	return int(c.num)
}

func (c *valueChunk) Compact() {
	if l := len(c.b); cap(c.b) > l+chunkCompactCapacityThreshold {
		buf := make([]byte, l)
		copy(buf, c.b)
		c.b = buf
	}
}

func (c *valueChunk) Appender() (*valueAppender, error) {
	return &valueAppender{
		c: c,
	}, nil
}

type valueAppender struct {
	c *valueChunk
}

func (a *valueAppender) Append(_ int64, v []byte) {
	if len(v) == 0 {
		v = []byte(" ")
	}

	buf := make([]byte, binary.MaxVarintLen64)
	size := buf[:binary.PutUvarint(buf, uint64(len(v)))]

	a.c.b = append(a.c.b, size...)
	a.c.b = append(a.c.b, v...)
	a.c.num++
}

func (c *valueChunk) Iterator(it Iterator) *valueIterator {
	if valueIter, ok := it.(*valueIterator); ok {
		//TODO: valueIter.Reset(c.b)
		return valueIter
	}

	if len(c.b) == 0 && len(c.compressed) != 0 {
		buf := &bytes.Buffer{}
		//r := snappy.NewReader(bytes.NewBuffer(c.compressed))
		r, _ := zstd.NewReader(bytes.NewBuffer(c.compressed)) // TODO handle error
		defer r.Close()
		_, _ = io.Copy(buf, r)
		c.b = buf.Bytes()
		c.compressed = nil
	}

	return &valueIterator{
		br:       bytes.NewReader(c.b),
		numTotal: c.num,
	}
}

type valueIterator struct {
	br       *bytes.Reader
	numTotal uint16
	err      error

	v       []byte
	numRead uint16
}

func (it *valueIterator) Next() bool {
	if it.err != nil || it.numRead == it.numTotal {
		return false
	}

	sampleLen, err := binary.ReadUvarint(it.br)
	if err != nil {
		it.err = err
		return false
	}

	it.v = make([]byte, sampleLen)
	_, err = it.br.Read(it.v)
	if err != nil {
		it.err = err
		return false
	}

	if bytes.Equal(it.v, []byte(" ")) {
		it.v = nil
	}

	it.numRead++
	return true
}

func (it *valueIterator) Seek(_ int64) bool {
	// TODO:
	// This is interesting. We don't know anything about timestamps here.
	// We could somehow translate timestamp to index?
	// We don't need this at all?
	panic("implement me")
}

func (it *valueIterator) At() (int64, []byte) {
	// timestamp is always 0 as ignored in this chunk
	return 0, it.v
}

func (it *valueIterator) Err() error {
	return it.err
}
