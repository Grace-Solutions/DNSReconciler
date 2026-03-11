package route53

import "encoding/xml"

// --- Request types ---

// changeResourceRecordSetsRequest is the root XML element for a change batch.
type changeResourceRecordSetsRequest struct {
	XMLName     xml.Name    `xml:"ChangeResourceRecordSetsRequest"`
	XMLNS       string      `xml:"xmlns,attr"`
	ChangeBatch changeBatch `xml:"ChangeBatch"`
}

type changeBatch struct {
	Comment string   `xml:"Comment,omitempty"`
	Changes []change `xml:"Changes>Change"`
}

type change struct {
	Action            string            `xml:"Action"`
	ResourceRecordSet resourceRecordSet `xml:"ResourceRecordSet"`
}

type resourceRecordSet struct {
	Name            string           `xml:"Name"`
	Type            string           `xml:"Type"`
	TTL             int              `xml:"TTL,omitempty"`
	ResourceRecords []resourceRecord `xml:"ResourceRecords>ResourceRecord,omitempty"`
}

type resourceRecord struct {
	Value string `xml:"Value"`
}

// --- Response types ---

type listResourceRecordSetsResponse struct {
	XMLName              xml.Name              `xml:"ListResourceRecordSetsResponse"`
	ResourceRecordSets   []resourceRecordSetXML `xml:"ResourceRecordSets>ResourceRecordSet"`
	IsTruncated          bool                  `xml:"IsTruncated"`
	NextRecordName       string                `xml:"NextRecordName"`
	NextRecordType       string                `xml:"NextRecordType"`
}

type resourceRecordSetXML struct {
	Name            string           `xml:"Name"`
	Type            string           `xml:"Type"`
	TTL             int              `xml:"TTL"`
	ResourceRecords []resourceRecord `xml:"ResourceRecords>ResourceRecord"`
}

type changeResourceRecordSetsResponse struct {
	XMLName    xml.Name   `xml:"ChangeResourceRecordSetsResponse"`
	ChangeInfo changeInfo `xml:"ChangeInfo"`
}

type changeInfo struct {
	ID        string `xml:"Id"`
	Status    string `xml:"Status"`
	Comment   string `xml:"Comment"`
}

type errorResponse struct {
	XMLName xml.Name `xml:"ErrorResponse"`
	Error   struct {
		Type    string `xml:"Type"`
		Code    string `xml:"Code"`
		Message string `xml:"Message"`
	} `xml:"Error"`
}

