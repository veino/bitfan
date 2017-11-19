//go:generate bitfanDoc -codec encoder
// doc codec
package linecodec

import (
	"bytes"
	"io"
	"text/template"

	"github.com/mitchellh/mapstructure"
	"github.com/vjeantet/bitfan/codecs/lib"
	"github.com/vjeantet/bitfan/core/location"
)

// doc encoder
type encoder struct {
	w         io.Writer
	options   encoderOptions
	formatTpl *template.Template

	log lib.Logger
}

// doc encoderOptions
type encoderOptions struct {
	// Change the delimiter that separates lines
	// @Default "\\n"
	Delimiter string

	// Format as a golang text/template
	// @Default "{{TS "dd/MM/yyyy:HH:mm:ss" .}} {{.host}} {{.message}}"
	Format string `mapstructure:"format"`

	// You can set variable to be used in Statements by using ${var}.
	// each reference will be replaced by the value of the variable found in Statement's content
	// The replacement is case-sensitive.
	// @ExampleLS var => {"hostname"=>"myhost","varname"=>"varvalue"}
	Var map[string]string `mapstructure:"var"`
}

func NewEncoder(w io.Writer) *encoder {
	e := &encoder{
		w: w,
		options: encoderOptions{
			Delimiter: "\n",
			Format:    `{{ TS "dd/MM/yyyy:HH:mm:ss" . }} {{.host}} {{.message}}`,
		},
	}

	return e
}

func (e *encoder) SetOptions(conf map[string]interface{}, logger lib.Logger, cwl string) error {
	e.log = logger

	if err := mapstructure.Decode(conf, &e.options); err != nil {
		return err
	}

	if e.options.Format != "" {

		//TODO : add a location.TemplateWithOptions to return golang text/template

		loc, err := location.NewLocation(e.options.Format, cwl)
		if err != nil {
			return err
		}

		e.formatTpl, _, err = loc.TemplateWithOptions(e.options.Var)
		if err != nil {
			return err
		}
	}

	return nil
}

func (e *encoder) Encode(data map[string]interface{}) error {
	buff := bytes.NewBufferString("")
	e.formatTpl.Execute(buff, data)
	e.w.Write(buff.Bytes())
	e.w.Write([]byte(e.options.Delimiter))
	return nil
}
