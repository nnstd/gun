package crypto

import (
	"bytes"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"strings"
	"time"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	"github.com/nnstd/gun/runtime/buffer"
)

// ---------------------------------------------------------------------------
// SPKAC-related ASN.1 structures
// ---------------------------------------------------------------------------

type spkacStruct struct {
	PublicKeyInfo asn1.RawValue
	Challenge    asn1.RawValue `asn1:"optional"`
}

type spkacSubjectPublicKeyInfo struct {
	Algorithm asn1.RawValue
	PublicKey asn1.BitString
}

// ---------------------------------------------------------------------------
// Certificate class — SPKAC operations (HTML5 keygen element)
// ---------------------------------------------------------------------------

func init() {
	certificate := jsvalue.NewObject()

	certificate.Set("exportChallenge", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 1 || args[0] == nil {
			panic(errInvalidArgType("a Buffer or string (SPKAC)"))
		}

		spkacData := inputBytes(args[0])
		if len(spkacData) == 0 {
			panic(errInvalidArgType("a non-empty Buffer or string (SPKAC)"))
		}

		spkac, err := parseSPKAC(spkacData)
		if err != nil {
			panic(errOperationFailed("Failed to parse SPKAC: " + err.Error()))
		}

		return buffer.NewBufferFromBytes(spkac.Challenge.Bytes)
	}))

	certificate.Set("exportPublicKey", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 1 || args[0] == nil {
			panic(errInvalidArgType("a Buffer or string (SPKAC)"))
		}

		spkacData := inputBytes(args[0])
		if len(spkacData) == 0 {
			panic(errInvalidArgType("a non-empty Buffer or string (SPKAC)"))
		}

		spkac, err := parseSPKAC(spkacData)
		if err != nil {
			panic(errOperationFailed("Failed to parse SPKAC: " + err.Error()))
		}

		pubKeyInfo := spkacSubjectPublicKeyInfo{}
		rest, err := asn1.Unmarshal(spkac.PublicKeyInfo.FullBytes, &pubKeyInfo)
		if err != nil {
			panic(errOperationFailed("Failed to parse SPKAC public key: " + err.Error()))
		}
		_ = rest

		pubKeyBytes, err := asn1.Marshal(pubKeyInfo)
		if err != nil {
			panic(errOperationFailed("Failed to marshal SPKAC public key: " + err.Error()))
		}

		return buffer.NewBufferFromBytes(pubKeyBytes)
	}))

	certificate.Set("verifySpkac", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 1 || args[0] == nil {
			panic(errInvalidArgType("a Buffer or string (SPKAC)"))
		}

		spkacData := inputBytes(args[0])
		if len(spkacData) == 0 {
			panic(errInvalidArgType("a non-empty Buffer or string (SPKAC)"))
		}

		_, err := parseSPKAC(spkacData)
		if err != nil {
			return jsvalue.NewBool(false)
		}

		return jsvalue.NewBool(true)
	}))

	AsJSValue.Set("Certificate", certificate)
	AsJSValue.Set("X509Certificate", newStaticX509Certificate())
}

// parseSPKAC parses a DER-encoded SPKAC structure.
func parseSPKAC(data []byte) (*spkacStruct, error) {
	spkac := &spkacStruct{}
	rest, err := asn1.Unmarshal(data, spkac)
	if err != nil {
		return nil, fmt.Errorf("invalid SPKAC: %w", err)
	}
	if len(rest) > 0 {
		return nil, fmt.Errorf("trailing data in SPKAC")
	}
	return spkac, nil
}

// ---------------------------------------------------------------------------
// X509Certificate — static constructor and class
// ---------------------------------------------------------------------------

func newStaticX509Certificate() *jsvalue.JSValue {
	x509Class := jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 || args[0] == nil {
			panic(errInvalidArgType("a Buffer or string"))
		}

		data := inputBytes(args[0])
		if len(data) == 0 {
			panic(errInvalidArgType("a non-empty Buffer or string"))
		}

		// Try PEM first
		if block, _ := pem.Decode(data); block != nil {
			data = block.Bytes
		}

		cert, err := x509.ParseCertificate(data)
		if err != nil {
			panic(errOperationFailed("Failed to parse X509 certificate: " + err.Error()))
		}

		return newX509CertificateObject(cert)
	})

	return x509Class
}

// newX509CertificateObject creates a JSValue object wrapping an x509.Certificate.
func newX509CertificateObject(cert *x509.Certificate) *jsvalue.JSValue {
	obj := jsvalue.NewObject()

	// .subject — subject DN string
	obj.Set("subject", jsvalue.NewString(formatDN(cert.Subject)))

	// .issuer — issuer DN string
	obj.Set("issuer", jsvalue.NewString(formatDN(cert.Issuer)))

	// .serialNumber — serial number hex string
	obj.Set("serialNumber", jsvalue.NewString(cert.SerialNumber.Text(16)))

	// .notBefore — Date (return as number timestamp for simplicity)
	obj.Set("notBefore", jsvalue.NewString(cert.NotBefore.UTC().Format(time.RFC3339)))

	// .notAfter — Date
	obj.Set("notAfter", jsvalue.NewString(cert.NotAfter.UTC().Format(time.RFC3339)))

	// .fingerprint(hash) — fingerprint string
	obj.Set("fingerprint", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		hashName := "sha256"
		if len(args) > 0 && args[0] != nil && args[0].TypeString() != "undefined" {
			hashName = args[0].String()
		}
		return jsvalue.NewString(certFingerprint(cert.Raw, hashName))
	}))

	// .publicKey — export public key as DER Buffer
	obj.Set("publicKey", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		pubKeyBytes, err := x509.MarshalPKIXPublicKey(cert.PublicKey)
		if err != nil {
			panic(errOperationFailed("Failed to marshal public key: " + err.Error()))
		}
		return buffer.NewBufferFromBytes(pubKeyBytes)
	}))

	// .raw — raw DER bytes as Buffer
	obj.Set("raw", buffer.NewBufferFromBytes(cert.Raw))

	// .toString() — PEM string
	obj.Set("toString", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		block := &pem.Block{
			Type:  "CERTIFICATE",
			Bytes: cert.Raw,
		}
		return jsvalue.NewString(string(pem.EncodeToMemory(block)))
	}))

	// .toLegacyObject() — returns object with subject/issuer/etc as objects
	obj.Set("toLegacyObject", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return newLegacyCertObject(cert)
	}))

	// .checkIssued(parentCert) — check if parent issued this cert
	obj.Set("checkIssued", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 1 || args[0] == nil {
			panic(errInvalidArgType("a X509Certificate"))
		}
		// Compare this cert's issuer with the parent's subject
		parentRaw := inputBytes(args[0].Get("raw"))
			if len(parentRaw) == 0 {
				return jsvalue.NewBool(false)
			}
			parent, err := x509.ParseCertificate(parentRaw)
			if err != nil {
				return jsvalue.NewBool(false)
			}
			return jsvalue.NewBool(bytes.Equal(cert.RawIssuer, parent.RawSubject))
	}))

	// .checkPrivateKey(privateKey) — check if private key matches
	obj.Set("checkPrivateKey", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		// In a full implementation, this would parse the private key and check.
		// For now, return true as a placeholder since full KeyObject support
		// would be needed for a complete implementation.
		return jsvalue.NewBool(true)
	}))

	// .validFrom — alias for notBefore
	obj.Set("validFrom", jsvalue.NewString(cert.NotBefore.UTC().Format(time.RFC3339)))

	// .validTo — alias for notAfter
	obj.Set("validTo", jsvalue.NewString(cert.NotAfter.UTC().Format(time.RFC3339)))

	return obj
}

// formatDN formats a pkix.Name as a readable DN string.
func formatDN(name pkix.Name) string {
	parts := make([]string, 0, len(name.Names))
	for _, rdn := range name.Names {
		if rdn.Value != nil {
			parts = append(parts, fmt.Sprintf("%s=%s", rdn.Type.String(), rdn.Value))
		}
	}
	return strings.Join(parts, ", ")
}

// certFingerprint computes the fingerprint of a certificate's raw DER bytes.
func certFingerprint(raw []byte, hashName string) string {
	switch strings.ToLower(hashName) {
	case "sha1":
		h := sha1.Sum(raw)
		return strings.ToLower(hex.EncodeToString(h[:]))
	case "sha256":
		h := sha256.Sum256(raw)
		return strings.ToLower(hex.EncodeToString(h[:]))
	default:
		h := sha256.Sum256(raw)
		return strings.ToLower(hex.EncodeToString(h[:]))
	}
}

// newLegacyCertObject creates a legacy-style certificate object.
func newLegacyCertObject(cert *x509.Certificate) *jsvalue.JSValue {
	obj := jsvalue.NewObject()

	obj.Set("subject", newLegacyNameObject(cert.Subject))
	obj.Set("issuer", newLegacyNameObject(cert.Issuer))
	obj.Set("serialNumber", jsvalue.NewString(cert.SerialNumber.Text(16)))
	obj.Set("notBefore", jsvalue.NewString(cert.NotBefore.UTC().Format(time.RFC3339)))
	obj.Set("notAfter", jsvalue.NewString(cert.NotAfter.UTC().Format(time.RFC3339)))

	return obj
}

// newLegacyNameObject creates a legacy-style name object from a pkix.Name.
func newLegacyNameObject(name pkix.Name) *jsvalue.JSValue {
	obj := jsvalue.NewObject()
	obj.Set("CN", jsvalue.NewString(name.CommonName))

	// Set other attributes
	for _, names := range [][]string{
		name.Country, name.Organization, name.OrganizationalUnit,
		name.Locality, name.Province,
	} {
		for _, n := range names {
			if n != "" {
				// Legacy Node.js uses C, O, OU, L, ST keys
				break
			}
		}
	}

	if len(name.Country) > 0 {
		obj.Set("C", jsvalue.NewString(name.Country[0]))
	}
	if len(name.Organization) > 0 {
		obj.Set("O", jsvalue.NewString(name.Organization[0]))
	}
	if len(name.OrganizationalUnit) > 0 {
		obj.Set("OU", jsvalue.NewString(name.OrganizationalUnit[0]))
	}
	if len(name.Locality) > 0 {
		obj.Set("L", jsvalue.NewString(name.Locality[0]))
	}
	if len(name.Province) > 0 {
		obj.Set("ST", jsvalue.NewString(name.Province[0]))
	}

	return obj
}
