//go:generate bitfanDoc -codec json_lines
package jsonlinescodec

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"

	"github.com/mitchellh/mapstructure"
	"github.com/vjeantet/bitfan/commons"
)

type decoder struct {
	more    bool
	r       *bufio.Scanner
	options decoderOptions

	log commons.Logger
}

type decoderOptions struct {
	// Change the delimiter that separates lines
	// @Default "\n"
	Delimiter string
}

func NewDecoder(r io.Reader) *decoder {
	d := &decoder{
		r:    bufio.NewScanner(r),
		more: true,
		options: decoderOptions{
			Delimiter: "\n",
		},
	}

	split := func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		// Return nothing if at end of file and no data passed
		if atEOF && len(data) == 0 {
			return 0, nil, nil
		}

		// Find the index of the input of a newline followed by a
		// pound sign.
		if i := strings.Index(string(data), d.options.Delimiter); i >= 0 {
			return i + 1, data[0:i], nil
		}

		// If at end of file with data return the data
		if atEOF {
			return len(data), data, nil
		}

		// Request more data.
		return 0, nil, nil
	}

	if d.options.Delimiter == "\n" {
		d.r.Split(bufio.ScanLines)
	} else {
		d.r.Split(split)
	}
	return d
}

func (d *decoder) SetOptions(conf map[string]interface{}, logger commons.Logger, cwl string) error {
	d.log = logger

	return mapstructure.Decode(conf, &d.options)
}

func (d *decoder) Decode(v *interface{}) error {

	if d.r.Scan() {
		d.more = true
		json.Unmarshal([]byte(d.r.Text()), v)
	} else {
		d.more = false
		return io.EOF
	}

	return nil
}

func (d *decoder) More() bool {
	return d.more
}
