package alfred

import (
	"fmt"
	"os"

	"howett.net/plist"
)

// Plist is a plist data structure
type Plist map[string]interface{}

// LoadPlist loads a plist from an XML file
func LoadPlist(filename string) (p Plist) {
	var err error
	var xmlData []byte
	if xmlData, err = os.ReadFile(filename); err != nil {
		panic(fmt.Errorf("error reading plist file: %s", err))
	}

	if _, err = plist.Unmarshal(xmlData, &p); err != nil {
		panic(err)
	}

	return
}

// SavePlist saves a plist to an XML file
func SavePlist(filename string, p Plist) {
	var err error
	var xmlData []byte
	if xmlData, err = plist.MarshalIndent(p, plist.XMLFormat, "\t"); err != nil {
		panic(fmt.Errorf("error serializing plist data: %s", err))
	}

	if err = os.WriteFile(filename, xmlData, 0644); err != nil {
		panic(err)
	}
}
