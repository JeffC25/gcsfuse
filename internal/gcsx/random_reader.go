// Copyright 2015 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gcsx

import (
	"errors"
	"fmt"
	"io"

	"github.com/jacobsa/gcloud/gcs"
	"golang.org/x/net/context"
)

// An object that knows how to read ranges within a particular generation of a
// particular GCS object. May make optimizations when it e.g. detects large
// sequential reads.
//
// Not safe for concurrent access.
type RandomReader interface {
	// Panic if any internal invariants are violated.
	CheckInvariants()

	// Matches the semantics of io.ReaderAt, with the addition of context
	// support.
	ReadAt(ctx context.Context, p []byte, offset int64) (n int, err error)

	// Return the record for the object to which the reader is bound.
	Object() (o *gcs.Object)

	// Clean up any resources associated with the reader, which must not be used
	// again.
	Destroy()
}

// Create a random reader for the supplied object record that reads using the
// given bucket.
func NewRandomReader(
	o *gcs.Object,
	bucket gcs.Bucket) (rr RandomReader, err error) {
	rr = &randomReader{
		object: o,
		bucket: bucket,
		start:  -1,
		limit:  -1,
	}

	return
}

type randomReader struct {
	object *gcs.Object
	bucket gcs.Bucket

	// If non-nil, an in-flight read request and a function for cancelling it.
	//
	// INVARIANT: (reader == nil) == (cancel == nil)
	reader io.ReadCloser
	cancel func()

	// The range of the object that we expect reader to yield, when reader is
	// non-nil. When reader is nil, limit is the limit of the previous read
	// operation, or -1 if there has never been one.
	//
	// INVARIANT: start <= limit
	// INVARIANT: limit < 0 implies reader != nil
	start int64
	limit int64
}

func (rr *randomReader) CheckInvariants() {
	// INVARIANT: (reader == nil) == (cancel == nil)
	if (rr.reader == nil) != (rr.cancel == nil) {
		panic(fmt.Sprintf("Mismatch: %v vs. %v", rr.reader, rr.cancel))
	}

	// INVARIANT: start <= limit
	if !(rr.start <= rr.limit) {
		panic(fmt.Sprintf("Unexpected range: [%d, %d)", rr.start, rr.limit))
	}

	// INVARIANT: limit < 0 implies reader != nil
	if rr.limit < 0 && rr.reader != nil {
		panic(fmt.Sprintf("Unexpected non-nil reader with limit == %d", rr.limit))
	}
}

func (rr *randomReader) ReadAt(
	ctx context.Context,
	p []byte,
	offset int64) (n int, err error) {
	for len(p) > 0 {
		// If we have an existing reader but it's positioned at the wrong place,
		// clean it up and throw it away.
		if rr.reader != nil && rr.start != offset {
			rr.reader.Close()
			rr.reader = nil
			rr.cancel = nil
		}

		// If we don't have a reader, start a read operation.
		if rr.reader == nil {
			err = rr.startRead(offset, len(p))
			if err != nil {
				err = fmt.Errorf("startRead: %v", err)
				return
			}
		}

		// Now we have a reader positioned at the correct place. Consume as much from
		// it as possible.
		var tmp int
		tmp, err = rr.readFull(ctx, p)

		n += tmp
		p = p[tmp:]
		rr.start += int64(tmp)

		// Sanity check.
		if rr.start > rr.limit {
			err = fmt.Errorf("Reader returned %d too many bytes", rr.start-rr.limit)

			// Don't attempt to reuse the reader when it's behaving wackily.
			rr.reader.Close()
			rr.reader = nil
			rr.cancel = nil
			rr.start = -1
			rr.limit = -1

			return
		}

		// Are we finished with this reader now?
		if rr.start == rr.limit {
			rr.reader.Close()
			rr.reader = nil
			rr.cancel = nil
		}

		// Handle errors.
		switch {
		case err == io.EOF || err == io.ErrUnexpectedEOF:
			// For a non-empty buffer, ReadFull returns EOF or ErrUnexpectedEOF only
			// if the reader peters out early. That's fine, but it means we should
			// have hit the limit above.
			if rr.reader != nil {
				err = fmt.Errorf("Reader returned %d too few bytes", rr.limit-rr.start)
				return
			}

			err = nil

		case err != nil:
			// Propagate other errors.
			err = fmt.Errorf("readFull: %v", err)
			return
		}
	}

	return
}

func (rr *randomReader) Object() (o *gcs.Object) {
	o = rr.object
	return
}

func (rr *randomReader) Destroy() {
	panic("TODO")
}

// Like io.ReadFull, but deals with the cancellation issues.
//
// REQUIRES: rr.reader != nil
func (rr *randomReader) readFull(
	ctx context.Context,
	p []byte) (n int, err error) {
	err = errors.New("TODO")
	return
}

// Ensure that rr.reader is set up for a range for which [start, start+size) is
// a prefix.
func (rr *randomReader) startRead(
	start int64,
	size int) (err error) {
	err = errors.New("TODO")
	return
}
