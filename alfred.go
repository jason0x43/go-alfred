package alfred

import (
	"log"
	"encoding/xml"
)

var MaxResults = 9

type AlfredResult struct {
	XMLName      xml.Name `xml:"item"`
	Uid          string   `xml:"uid,attr,omitempty"`
	Arg          string   `xml:"arg,omitempty"`
	Title        string   `xml:"title"`
	SubTitle     string   `xml:"subtitle,omitempty"`
	Icon         string   `xml:"icon,omitempty"`
	Valid        bool     `xml:"valid,attr"`
	AutoComplete string   `xml:"autocomplete,attr,omitempty"`
}

func ToXML(results []AlfredResult) string {
	newxml := "<items>"

	for i := 0; i < len(results); i++ {
		data, err := xml.Marshal(results[i])
		if err != nil {
			log.Fatalf("ToXML Error: %v\n", err)
		}
		newxml += string(data)
	}

	newxml += "</items>"
	return newxml
}
