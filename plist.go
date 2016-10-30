package alfred

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"time"
)

// Decoder is a Plist decoder
type Decoder struct {
	xd *xml.Decoder
}

// Plist represents a property list.
type Plist struct {
	Version string
	Root    interface{}
}

// Dict represents a property list "dict" element.
type Dict map[string]interface{}

// Array represents a property list "array" element.
type Array []interface{}

// Unmarshal parses a Plist out of a given array of bytes.
func Unmarshal(data []byte, p *Plist) error {
	dec := Decoder{xml.NewDecoder(bytes.NewBuffer(data))}
	return dec.Decode(p)
}

// UnmarshalFile loads a file and parses a Plist from the loaded data.
// If the file is a binary plist, the plutil system command is used to convert
// it to XML text.
func UnmarshalFile(filename string) (plist Plist, err error) {
	var xmlFile *os.File
	if xmlFile, err = os.Open(filename); err != nil {
		return
	}
	defer xmlFile.Close()

	var xmlData []byte
	if xmlData, err = ioutil.ReadAll(xmlFile); err != nil {
		return
	}

	err = Unmarshal(xmlData, &plist)
	return
}

// Decode executes the Decoder to fill in a Plist structure.
func (d *Decoder) Decode(plist *Plist) (err error) {
	var start xml.StartElement
	if start, err = d.decodeStartElement("plist"); err != nil {
		return
	}

	if len(start.Attr) != 1 || start.Attr[0].Name.Local != "version" {
		err = fmt.Errorf("plist: missing version")
		return
	}
	plist.Version = start.Attr[0].Value

	// Start element of plist content
	var se xml.StartElement
	var isEnd bool
	if se, _, isEnd, err = d.decodeStartOrEndElement("", start); err != nil {
		return
	}

	if isEnd {
		// empty plist
		dlog.Printf("empty plist")
		return
	}

	switch se.Name.Local {
	case "dict":
		d, err := d.decodeDict(se)
		if err != nil {
			return err
		}
		dlog.Printf("read dict: %s", d)
		plist.Root = d
	case "array":
		a, err := d.decodeArray(se)
		if err != nil {
			return err
		}
		dlog.Printf("read array: %s", a)
		plist.Root = a
	default:
		return fmt.Errorf("plist: bad root element: must be dict or array")
	}

	return err
}

func (d *Decoder) decodeStartElement(expected string) (start xml.StartElement, err error) {
	dlog.Printf("reading start element '%s'", expected)

	var t xml.Token
	if t, err = d.nextElement(); err != nil {
		return
	}

	var ok bool
	if start, ok = t.(xml.StartElement); !ok {
		err = fmt.Errorf("plist: expected StartElement, saw %T", t)
	} else if expected != "" && start.Name.Local != expected {
		err = fmt.Errorf("plist: unexpected key name '%s'", start.Name.Local)
	}

	return
}

func (d *Decoder) decodeStartOrEndElement(expected string, s xml.StartElement) (start xml.StartElement, end xml.EndElement, isEnd bool, err error) {
	dlog.Printf("reading start element '%s' or end element", expected)

	var t xml.Token
	if t, err = d.nextElement(); err != nil {
		return
	}

	var ok bool
	if start, ok = t.(xml.StartElement); !ok {
		if end, ok = t.(xml.EndElement); ok {
			if end.Name.Local == s.Name.Local {
				dlog.Printf("  read end element '%s'", end)
				isEnd = true
				return
			}
			err = fmt.Errorf("plist: unexpected end element '%s'", end.Name.Local)
			return
		}
		err = fmt.Errorf("plist: expected StartElement, saw %T", start)
		return
	}

	if expected != "" && start.Name.Local != expected {
		err = fmt.Errorf("plist: unexpected key name '%s'", start.Name.Local)
		return
	}

	dlog.Printf("  read start element '%#v'", start)
	return
}

func (d *Decoder) decodeDict(start xml.StartElement) (dict Dict, err error) {
	dlog.Printf("reading dict")

	// <key>
	var se xml.StartElement
	var isEnd bool
	if se, _, isEnd, err = d.decodeStartOrEndElement("key", start); err != nil {
		return
	}

	if isEnd {
		// empty dict
		return
	}

	dict = make(Dict)

	for {
		// read key name
		var keyName string
		if keyName, err = d.decodeString(se); err != nil {
			return
		}

		// read start element
		if se, err = d.decodeStartElement(""); err != nil {
			return
		}

		// decode the element value
		var val interface{}
		if val, err = d.decodeValue(se); err != nil {
			return
		}

		dict[keyName] = val
		dlog.Printf("set dict[%s] = %#v", keyName, val)

		// get the next key
		if se, _, isEnd, err = d.decodeStartOrEndElement("key", start); err != nil {
			return
		}

		if isEnd {
			// end of list
			break
		}
	}

	dlog.Printf("filled in dict: %s", dict)
	return
}

func (d *Decoder) decodeArray(start xml.StartElement) (arr Array, err error) {
	dlog.Printf("reading array")

	var se xml.StartElement
	var isEnd bool
	if se, _, isEnd, err = d.decodeStartOrEndElement("", start); err != nil {
		return
	}

	if isEnd {
		// empty array
		return
	}

	for {
		// decode the current value
		var val interface{}
		if val, err = d.decodeValue(se); err != nil {
			return
		}

		arr = append(arr, val)

		// get the next value
		if se, _, isEnd, err = d.decodeStartOrEndElement("", start); err != nil {
			return
		}

		if isEnd {
			// end of array
			break
		}
	}

	return
}

func (d *Decoder) decodeAny(start xml.StartElement) (tok xml.Token, err error) {
	if tok, err = d.xd.Token(); err != nil {
		return
	}

	if end, ok := tok.(xml.EndElement); ok {
		if end.Name.Local != start.Name.Local {
			err = fmt.Errorf("plist: unexpected end tag: %s", end.Name.Local)
			return
		}
		// empty
		return
	}

	tok = xml.CopyToken(tok)

	var next xml.Token
	if next, err = d.nextElement(); err != nil {
		return
	}

	if end, ok := next.(xml.EndElement); !ok || end.Name.Local != start.Name.Local {
		// empty
		err = fmt.Errorf("plist: unexpected end tag: %s", end.Name.Local)
		return
	}

	return
}

func (d *Decoder) decodeString(start xml.StartElement) (str string, err error) {
	dlog.Printf("decoding string in <%s>", start.Name)

	var t xml.Token
	if t, err = d.decodeAny(start); err != nil {
		return
	}

	if t == nil {
		dlog.Printf("read empty string")
		return
	}

	var ee xml.EndElement
	var ok bool
	if ee, ok = t.(xml.EndElement); ok {
		if ee.Name == start.Name {
			dlog.Printf("read empty string")
			return
		}
		err = fmt.Errorf("plist: unexpected end tag %v", ee)
		return
	}

	dlog.Printf("read token '%#v'", t)

	var cd xml.CharData
	if cd, ok = t.(xml.CharData); !ok {
		err = fmt.Errorf("plist: expected character data")
		return
	}

	dlog.Printf("read string '%s'", string(cd))

	return string(cd), nil
}

func (d *Decoder) decodeNonEmptyString(start xml.StartElement) (str string, err error) {
	if str, err = d.decodeString(start); err != nil {
		return
	}

	if len(str) == 0 {
		err = fmt.Errorf("plist: expected non-empty string")
	}

	return
}

func (d *Decoder) decodeDate(start xml.StartElement) (t time.Time, err error) {
	var str string
	if str, err = d.decodeNonEmptyString(start); err != nil {
		return
	}

	dlog.Printf("read date: %s", str)

	return time.Parse(time.RFC3339, str)
}

func (d *Decoder) decodeData(start xml.StartElement) (data []byte, err error) {
	dlog.Printf("reading data")
	var str string
	if str, err = d.decodeString(start); err != nil || str == "" {
		return
	}

	dlog.Printf("read data: %s", str)

	return base64.StdEncoding.DecodeString(str)
}

func (d *Decoder) decodeInteger(start xml.StartElement) (i int64, err error) {
	var str string
	if str, err = d.decodeNonEmptyString(start); err != nil {
		return
	}

	dlog.Printf("read integer: %s", str)

	return strconv.ParseInt(str, 10, 64)
}

func (d *Decoder) decodeReal(start xml.StartElement) (f float64, err error) {
	var str string
	if str, err = d.decodeNonEmptyString(start); err != nil {
		return
	}

	dlog.Printf("read real: %s", str)

	return strconv.ParseFloat(str, 64)
}

func (d *Decoder) decodeValue(se xml.StartElement) (val interface{}, err error) {
	switch se.Name.Local {
	case "dict":
		val, err = d.decodeDict(se)
	case "array":
		val, err = d.decodeArray(se)
	case "true":
		val = true
		_, err = d.nextElement()
	case "false":
		val = false
		_, err = d.nextElement()
	case "date":
		val, err = d.decodeDate(se)
	case "data":
		val, err = d.decodeData(se)
	case "string":
		val, err = d.decodeString(se)
	case "real":
		val, err = d.decodeReal(se)
	case "integer":
		val, err = d.decodeInteger(se)
	}

	return
}

func (d *Decoder) nextElement() (tok xml.Token, err error) {
	for {
		if tok, err = d.xd.Token(); err != nil {
			return
		}

		switch tok.(type) {
		case xml.StartElement, xml.EndElement:
			return
		}
	}
}
