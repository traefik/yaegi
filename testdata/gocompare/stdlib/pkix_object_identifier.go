// crypto/x509/pkix with an asn1.ObjectIdentifier made by the program: with
// -aot it is a value of the copy of encoding/asn1, stored in a field of the
// native pkix types.
package main

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"fmt"
)

func main() {
	ou := asn1.ObjectIdentifier{2, 5, 4, 11}
	n := pkix.Name{ExtraNames: []pkix.AttributeTypeAndValue{{Type: ou, Value: "unit"}}}
	fmt.Println(n.String())
	fmt.Println(n.ExtraNames[0].Type.Equal(ou), n.ExtraNames[0].Type)

	seq := pkix.RDNSequence{
		{{Type: asn1.ObjectIdentifier{2, 5, 4, 3}, Value: "name"}},
		{{Type: ou, Value: "unit"}},
	}
	var m pkix.Name
	m.FillFromRDNSequence(&seq)
	fmt.Printf("%q %q %d %s\n", m.CommonName, m.OrganizationalUnit, len(m.Names), seq.String())

	atv := pkix.AttributeTypeAndValue{}
	atv.Type = ou
	fmt.Println(atv.Type.String())

	// slices of asn1.ObjectIdentifier in the fields of x509.Certificate
	var c x509.Certificate
	c.PolicyIdentifiers = []asn1.ObjectIdentifier{{1, 2, 3}}
	c.UnknownExtKeyUsage = []asn1.ObjectIdentifier{{1, 3, 6}, ou}
	c.PolicyIdentifiers = append(c.PolicyIdentifiers, ou)
	fmt.Println(c.PolicyIdentifiers, c.UnknownExtKeyUsage, c.PolicyIdentifiers[1].Equal(ou))
	var empty []asn1.ObjectIdentifier
	c.UnknownExtKeyUsage = empty
	fmt.Println(c.UnknownExtKeyUsage == nil, len(c.UnknownExtKeyUsage))
}
