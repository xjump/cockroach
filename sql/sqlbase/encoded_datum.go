// Copyright 2016 The Cockroach Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
// implied. See the License for the specific language governing
// permissions and limitations under the License.
//
// Author: Radu Berinde (radu@cockroachlabs.com)

package sqlbase

import (
	"fmt"

	"github.com/cockroachdb/cockroach/sql/parser"
	"github.com/cockroachdb/cockroach/util"
	"github.com/cockroachdb/cockroach/util/encoding"
)

// DatumEncoding identifies the encoding used for an EncDatum.
type DatumEncoding int32

// Possible encodings.
const (
	NoEncoding DatumEncoding = iota
	AscendingKeyEncoding
	DescendingKeyEncoding
	// ValueEncoding to be supported later.
)

// EncDatum represents a datum that is "backed" by an encoding and/or by a
// parser.Datum. It allows "passing through" a Datum without decoding and
// reencoding. TODO(radu): It will also allow comparing encoded datums directly
// (for certain encodings).
type EncDatum struct {
	Type ColumnType_Kind

	// Encoding type. NoEncoding iff encoded is nil.
	encoding DatumEncoding

	// Encoded datum (according to the encoding field).
	encoded []byte

	// Decoded datum.
	Datum parser.Datum
}

// SetEncoded initializes the EncDatum with the given encoded value. The encoded
// value is stored as a shallow copy, so the caller must make sure the slice is
// not modified for the lifetime of the EncDatum.
func (ed *EncDatum) SetEncoded(typ ColumnType_Kind, enc DatumEncoding, val []byte) {
	if val == nil {
		panic("nil encoded value given")
	}
	ed.Type = typ
	ed.encoding = enc
	ed.encoded = val
	ed.Datum = nil
}

// SetDatum initializes the EncDatum with the given Datum.
func (ed *EncDatum) SetDatum(typ ColumnType_Kind, d parser.Datum) {
	if d == nil {
		panic("nil datum given")
	}
	if d != parser.DNull && !typ.ToDatumType().TypeEqual(d) {
		panic(fmt.Sprintf("invalid datum type given: %s, expected %s",
			d.Type(), typ.ToDatumType().Type()))
	}
	ed.Type = typ
	ed.encoding = NoEncoding
	ed.encoded = nil
	ed.Datum = d
}

// IsUnset returns true if SetEncoded or SetDatum were not called.
func (ed *EncDatum) IsUnset() bool {
	return ed.encoded == nil && ed.Datum == nil
}

// Decode ensures that Datum is set (decoding if necessary).
func (ed *EncDatum) Decode(a *DatumAlloc) error {
	if ed.Datum != nil {
		return nil
	}
	datType := ed.Type.ToDatumType()
	var err error
	var rem []byte
	switch ed.encoding {
	case AscendingKeyEncoding:
		ed.Datum, rem, err = DecodeTableKey(a, datType, ed.encoded, encoding.Ascending)
	case DescendingKeyEncoding:
		ed.Datum, rem, err = DecodeTableKey(a, datType, ed.encoded, encoding.Descending)
	case NoEncoding:
		panic("EncDatum without datum or encoding")
	default:
		panic(fmt.Sprintf("invalid encoding %d", ed.encoding))
	}
	if len(rem) != 0 {
		return util.Errorf("%d trailing bytes in encoded value", len(rem))
	}
	return err
}

// Encoding returns the encoding that is already available
// (NoEncoding if none).
func (ed *EncDatum) Encoding() DatumEncoding {
	return ed.encoding
}

// Encode appends the encoded datum to the given slice using the requested
// encoding.
func (ed *EncDatum) Encode(a *DatumAlloc, enc DatumEncoding, appendTo []byte) ([]byte, error) {
	if enc == NoEncoding {
		panic("NoEncoding requested")
	}
	if enc == ed.encoding {
		// We already have an encoding that matches
		return append(appendTo, ed.encoded...), nil
	}
	if err := ed.Decode(a); err != nil {
		return nil, err
	}
	switch enc {
	case AscendingKeyEncoding:
		return EncodeTableKey(appendTo, ed.Datum, encoding.Ascending)
	case DescendingKeyEncoding:
		return EncodeTableKey(appendTo, ed.Datum, encoding.Descending)
	default:
		panic(fmt.Sprintf("invalid encoding requested %d", enc))
	}
}
