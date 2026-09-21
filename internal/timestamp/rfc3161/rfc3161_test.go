package rfc3161

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1" // #nosec G505 -- test locates the RFC 2634 ESSCertID bytes.
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestOpenSSLFixtureVerifiesLocally(t *testing.T) {
	target, request, response, root := fixture(t)
	requestInfo, err := ParseRequest(request, target, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if requestInfo.HashAlgorithm != HashAlgorithm || !requestInfo.HasNonce || !requestInfo.RequestsCert {
		t.Fatalf("request info = %#v", requestInfo)
	}
	metadata, err := Verify(t.Context(), target, request, response, root, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if metadata.HashAlgorithm != HashAlgorithm || metadata.PolicyOID != "1.3.6.1.4.1.55555.1" || metadata.SerialNumber == "" || metadata.GenTime.IsZero() || metadata.SignerFingerprint == "" || metadata.CABundleSHA256 == "" {
		t.Fatalf("metadata = %#v", metadata)
	}
	if err := MetadataMatches(metadata, metadata.GenTime.Format(time.RFC3339Nano), metadata.PolicyOID, metadata.SerialNumber, HashAlgorithm); err != nil {
		t.Fatal(err)
	}
	if err := MetadataMatches(metadata, metadata.GenTime.Format(time.RFC3339Nano), metadata.PolicyOID, metadata.SerialNumber+"0", HashAlgorithm); SafeReason(err) != ReasonMetadata {
		t.Fatalf("metadata mismatch = %v (%s)", err, SafeReason(err))
	}
}

func TestFreeTSAESSV1SHA512FixtureVerifiesLocally(t *testing.T) {
	target, request, response, root := fixtureFromDirectory(t, "freetsa", "target.txt", "request.tsq", "response.tsr", filepath.Join("..", "..", "providers", "freetsa", "ca.pem"))
	metadata, err := Verify(t.Context(), target, request, response, root, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if metadata.HashAlgorithm != HashAlgorithm || metadata.PolicyOID != "1.2.3.4.1" || metadata.SerialNumber != "139111566" || metadata.GenTime.Format(time.RFC3339) != "2026-09-21T15:24:45Z" {
		t.Fatalf("metadata = %#v", metadata)
	}
	if metadata.SignerFingerprint != "32e841a95cc1164101ffde41298ef2fc75c1c4372ef095e88a6bbd47dfb191fc" || metadata.CABundleSHA256 != "2151b61137ffa86bf664691ba67e7da0b19f98c758e3d228d5d8ebf27e044438" {
		t.Fatalf("retained trust metadata = %#v", metadata)
	}
}

func TestFreeTSAFixtureClassifiesESSDigestAndAlgorithmMutations(t *testing.T) {
	target, request, response, root := fixtureFromDirectory(t, "freetsa", "target.txt", "request.tsq", "response.tsr", filepath.Join("..", "..", "providers", "freetsa", "ca.pem"))
	signerPEM, err := os.ReadFile(filepath.Join("testdata", "freetsa", "tsa.pem"))
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(signerPEM)
	if block == nil {
		t.Fatal("FreeTSA signer PEM is malformed")
	}
	signer, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	identifier := sha1.Sum(signer.Raw) // #nosec G401 -- RFC 2634 ESSCertID fixture locator.
	offset := bytes.Index(response, identifier[:])
	if offset < 0 {
		t.Fatal("ESS v1 certificate identifier was not found")
	}
	badESS := append([]byte(nil), response...)
	badESS[offset] ^= 0x01
	if _, err := Verify(t.Context(), target, request, badESS, root, DefaultLimits()); SafeReason(err) != ReasonSignerBinding {
		t.Fatalf("ESS mutation = %v (%s)", err, SafeReason(err))
	}

	parsed, err := parseResponse(response, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	token, err := parsed.SignedToken()
	if err != nil {
		t.Fatal(err)
	}
	var cmsDigest []byte
	for _, attribute := range token.SignerInfos[0].SignedAttributes {
		if attribute.Type.Equal(oidMessageDigest) {
			if err := unmarshalSingleAttribute(attribute.Values.Bytes, &cmsDigest); err != nil {
				t.Fatal(err)
			}
		}
	}
	offset = bytes.Index(response, cmsDigest)
	if offset < 0 {
		t.Fatal("CMS message digest was not found")
	}
	badDigest := append([]byte(nil), response...)
	badDigest[offset] ^= 0x01
	if _, err := Verify(t.Context(), target, request, badDigest, root, DefaultLimits()); SafeReason(err) != ReasonSignature {
		t.Fatalf("CMS digest mutation = %v (%s)", err, SafeReason(err))
	}

	sha512DER, _ := asn1.Marshal(oidSHA512)
	unknownOID := append([]byte(nil), sha512DER...)
	unknownOID[len(unknownOID)-1] = 0x7f
	badAlgorithm := bytes.ReplaceAll(response, sha512DER, unknownOID)
	if bytes.Equal(badAlgorithm, response) {
		t.Fatal("SHA-512 algorithm identifier was not found")
	}
	if _, err := Verify(t.Context(), target, request, badAlgorithm, root, DefaultLimits()); SafeReason(err) != ReasonAlgorithm {
		t.Fatalf("algorithm mutation = %v (%s)", err, SafeReason(err))
	}
}

func TestFixtureAlsoVerifiesWithOpenSSL(t *testing.T) {
	if _, err := exec.LookPath("openssl"); err != nil {
		t.Skip("OpenSSL is not installed")
	}
	command := exec.Command("openssl", "ts", "-verify", "-queryfile", "testdata/request.tsq", "-in", "testdata/response.tsr", "-CAfile", "testdata/root.pem")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("OpenSSL verification failed: %v: %s", err, output)
	}
}

func TestRequestProfileBindingBoundsAndTrailingData(t *testing.T) {
	target := []byte("deterministic target")
	entropy := bytes.NewReader(bytes.Repeat([]byte{0x42}, defaultNonceByteSize))
	request, info, err := CreateRequest(target, entropy, DefaultLimits())
	if err != nil || !info.HasNonce || !info.RequestsCert {
		t.Fatalf("CreateRequest = %#v, %v", info, err)
	}
	if _, err := ParseRequest(request, target, DefaultLimits()); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseRequest(request, []byte("other"), DefaultLimits()); SafeReason(err) != ReasonTargetMismatch {
		t.Fatalf("target mismatch = %v (%s)", err, SafeReason(err))
	}
	if _, err := ParseRequest(append(append([]byte(nil), request...), 0), target, DefaultLimits()); SafeReason(err) != ReasonRequestMalformed {
		t.Fatalf("trailing request = %v (%s)", err, SafeReason(err))
	}
	if _, _, err := CreateRequest(target, bytes.NewReader(nil), DefaultLimits()); SafeReason(err) != ReasonEntropy {
		t.Fatalf("entropy failure = %v (%s)", err, SafeReason(err))
	}
	limits := DefaultLimits()
	limits.TargetBytes = len(target) - 1
	if _, _, err := CreateRequest(target, rand.Reader, limits); SafeReason(err) != ReasonLimit {
		t.Fatalf("target limit = %v (%s)", err, SafeReason(err))
	}
}

func TestResponseRejectsTrailingBindingTargetAndTrustFailures(t *testing.T) {
	target, request, response, root := fixture(t)
	if err := ParseResponse(append(append([]byte(nil), response...), 0), DefaultLimits()); SafeReason(err) != ReasonResponseMalformed {
		t.Fatalf("trailing response = %v (%s)", err, SafeReason(err))
	}
	otherRequest, _, err := CreateRequest(target, bytes.NewReader(bytes.Repeat([]byte{0x24}, defaultNonceByteSize)), DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(t.Context(), target, otherRequest, response, root, DefaultLimits()); SafeReason(err) != ReasonNonceMismatch {
		t.Fatalf("nonce mismatch = %v (%s)", err, SafeReason(err))
	}
	if _, err := Verify(t.Context(), []byte("different target"), request, response, root, DefaultLimits()); SafeReason(err) != ReasonTargetMismatch {
		t.Fatalf("target mismatch = %v (%s)", err, SafeReason(err))
	}
	_, _, _, wrongRoot := fixtureFrom(t, "target.txt", "request.tsq", "response.tsr", "wrong-root.pem")
	if _, err := Verify(t.Context(), target, request, response, wrongRoot, DefaultLimits()); SafeReason(err) != ReasonTrustBundle {
		t.Fatalf("wrong trust root = %v (%s)", err, SafeReason(err))
	}
	if _, err := Verify(t.Context(), target, request, response, []byte("not PEM"), DefaultLimits()); SafeReason(err) != ReasonTrustBundle {
		t.Fatalf("invalid trust bundle = %v (%s)", err, SafeReason(err))
	}
}

func TestCertificateAndAlgorithmProfileHelpers(t *testing.T) {
	_, _, response, _ := fixture(t)
	parsed, err := parseResponse(response, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	token, err := parsed.SignedToken()
	if err != nil {
		t.Fatal(err)
	}
	info, err := token.Info()
	if err != nil {
		t.Fatal(err)
	}
	chains, err := token.Verify(t.Context(), x509.VerifyOptions{Roots: fixtureRoots(t), CurrentTime: info.GenTime, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageTimeStamping}})
	if err != nil || len(chains) == 0 || !hasCriticalTimestampingEKU(chains[0]) {
		t.Fatalf("fixture signer profile = %v, chains=%d", err, len(chains))
	}

	extension := pkix.Extension{Id: oidExtendedKeyUsage, Critical: true, Value: []byte{0x30, 0}}
	valid := &x509.Certificate{ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageTimeStamping}, Extensions: []pkix.Extension{extension}}
	if !hasCriticalTimestampingEKU(valid) {
		t.Fatal("critical timestamping EKU was rejected")
	}
	for name, certificate := range map[string]*x509.Certificate{
		"non-critical": {ExtKeyUsage: valid.ExtKeyUsage, Extensions: []pkix.Extension{{Id: oidExtendedKeyUsage}}},
		"extra usage":  {ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageTimeStamping, x509.ExtKeyUsageServerAuth}, Extensions: valid.Extensions},
		"unknown":      {ExtKeyUsage: valid.ExtKeyUsage, UnknownExtKeyUsage: []asn1.ObjectIdentifier{{1, 2, 3}}, Extensions: valid.Extensions},
	} {
		if hasCriticalTimestampingEKU(certificate) {
			t.Errorf("%s signer profile succeeded", name)
		}
	}

	weakRSA, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	strongRSA, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	if strongPublicKey(&x509.Certificate{PublicKey: &weakRSA.PublicKey}) || !strongPublicKey(&x509.Certificate{PublicKey: &strongRSA.PublicKey}) {
		t.Fatal("RSA strength profile mismatch")
	}
	if strongPublicKey(&x509.Certificate{PublicKey: &ecdsa.PublicKey{Curve: elliptic.P224()}}) || !strongPublicKey(&x509.Certificate{PublicKey: &ecdsa.PublicKey{Curve: elliptic.P256()}}) {
		t.Fatal("ECDSA strength profile mismatch")
	}
	if !weakSignature(x509.SHA1WithRSA) || weakSignature(x509.SHA256WithRSA) {
		t.Fatal("signature algorithm profile mismatch")
	}

	for _, test := range []struct {
		digest, signature asn1.ObjectIdentifier
	}{
		{oidSHA256, oidSHA256RSA},
		{oidSHA384, oidSHA384RSA},
		{oidSHA512, oidSHA512RSA},
		{oidSHA256, oidECDSASHA256},
		{oidSHA384, oidECDSASHA384},
		{oidSHA512, oidECDSASHA512},
	} {
		if _, algorithm, err := strongSignerAlgorithms(test.digest, test.signature); err != nil || algorithm == x509.UnknownSignatureAlgorithm {
			t.Errorf("strong signer algorithms %v/%v = %v, %v", test.digest, test.signature, algorithm, err)
		}
	}
	for _, digest := range []asn1.ObjectIdentifier{{1, 3, 14, 3, 2, 26}, {1, 2, 840, 113549, 2, 5}} {
		if _, _, err := strongSignerAlgorithms(digest, oidRSAEncryption); SafeReason(err) != ReasonAlgorithm {
			t.Errorf("weak digest %v = %v (%s)", digest, err, SafeReason(err))
		}
	}
}

func fixtureRoots(t *testing.T) *x509.CertPool {
	t.Helper()
	_, _, _, root := fixture(t)
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(root) {
		t.Fatal("fixture root could not be loaded")
	}
	return pool
}

func TestConstrainedHTTPClient(t *testing.T) {
	_, request, response, _ := fixture(t)
	resolver := staticResolver{addresses: []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}}
	transport := roundTripper(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost || req.Header.Get("Content-Type") != "application/timestamp-query" || req.Header.Get("Accept") != "application/timestamp-reply" {
			t.Fatalf("request = %s headers=%v", req.Method, req.Header)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/timestamp-reply"}}, Body: io.NopCloser(bytes.NewReader(response))}, nil
	})
	result, err := (HTTPClient{Resolver: resolver, Client: &http.Client{Transport: transport}}).Submit(t.Context(), "https://TSA.example.test/stamp", request)
	if err != nil || result.RequestCount != 1 || result.TSAOrigin != "https://tsa.example.test:443" || !bytes.Equal(result.Response, response) {
		t.Fatalf("Submit = %#v, %v", result, err)
	}
	for _, endpoint := range []string{"http://tsa.example.test", "https://user@tsa.example.test", "https://tsa.example.test:8443", "https://tsa.example.test/?secret=value", "https://127.0.0.1"} {
		client := HTTPClient{Resolver: staticResolver{addresses: []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}}, Client: &http.Client{Transport: transport}}
		if _, err := client.Submit(t.Context(), endpoint, request); err == nil {
			t.Errorf("unsafe endpoint %q succeeded", endpoint)
		}
	}
}

func TestHTTPRedirectAndResponseLimitsAreEnforced(t *testing.T) {
	_, request, response, _ := fixture(t)
	resolver := staticResolver{addresses: []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}}
	redirect := roundTripper(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": []string{"https://other.example.test/stamp"}}, Body: http.NoBody, Request: req}, nil
	})
	if _, err := (HTTPClient{Resolver: resolver, Client: &http.Client{Transport: redirect}}).Submit(t.Context(), "https://tsa.example.test/stamp", request); err == nil {
		t.Fatal("cross-origin redirect succeeded")
	}
	tooLarge := append(append([]byte(nil), response...), bytes.Repeat([]byte{0}, 32)...)
	limits := DefaultLimits()
	limits.ResponseBytes = len(response)
	oversize := roundTripper(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/timestamp-reply"}}, Body: io.NopCloser(bytes.NewReader(tooLarge))}, nil
	})
	if _, err := (HTTPClient{Resolver: resolver, Client: &http.Client{Transport: oversize}, Limits: limits}).Submit(t.Context(), "https://tsa.example.test/stamp", request); SafeReason(err) != ReasonLimit {
		t.Fatalf("oversize response = %v (%s)", err, SafeReason(err))
	}
}

func TestHTTPProfileRejectsPrivateResolutionAndUnsafeResponses(t *testing.T) {
	_, request, response, _ := fixture(t)
	for _, address := range []string{"127.0.0.1", "10.0.0.1", "169.254.1.1", "192.0.2.1", "::1", "2001:db8::1"} {
		client := HTTPClient{Resolver: staticResolver{addresses: []net.IPAddr{{IP: net.ParseIP(address)}}}}
		if _, err := client.Submit(t.Context(), "https://tsa.example.test/stamp", request); SafeReason(err) != ReasonRequestProfile {
			t.Errorf("private/reserved address %s = %v (%s)", address, err, SafeReason(err))
		}
	}

	publicResolver := staticResolver{addresses: []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}}
	tests := []struct {
		name   string
		status int
		type_  string
		body   io.ReadCloser
		reason Reason
	}{
		{name: "status", status: http.StatusServiceUnavailable, type_: "application/timestamp-reply", body: io.NopCloser(bytes.NewReader(response)), reason: ReasonHTTPStatus},
		{name: "media type", status: http.StatusOK, type_: "application/octet-stream", body: io.NopCloser(bytes.NewReader(response)), reason: ReasonMediaType},
		{name: "empty", status: http.StatusOK, type_: "application/timestamp-reply", body: http.NoBody, reason: ReasonLimit},
		{name: "truncated", status: http.StatusOK, type_: "application/timestamp-reply", body: io.NopCloser(bytes.NewReader(response[:len(response)/2])), reason: ReasonResponseMalformed},
		{name: "read", status: http.StatusOK, type_: "application/timestamp-reply", body: failingReadCloser{}, reason: ReasonTransport},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport := roundTripper(func(req *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: test.status, Header: http.Header{"Content-Type": []string{test.type_}}, Body: test.body, Request: req}, nil
			})
			_, err := (HTTPClient{Resolver: publicResolver, Client: &http.Client{Transport: transport}}).Submit(t.Context(), "https://tsa.example.test/stamp", request)
			if SafeReason(err) != test.reason {
				t.Fatalf("unsafe response = %v (%s); want %s", err, SafeReason(err), test.reason)
			}
		})
	}

	endpoint, _ := url.Parse("https://tsa.example.test/stamp")
	client := (HTTPClient{}).productionClient(endpoint, []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, false)
	transport, ok := client.Transport.(*http.Transport)
	if !ok || transport.Proxy != nil {
		t.Fatal("production timestamp transport inherited a proxy")
	}
}

func TestBuiltInTransportProfilesAllowExactHTTPAndRejectAllRedirects(t *testing.T) {
	_, request, response, _ := fixture(t)
	resolver := staticResolver{addresses: []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}}
	profile, ok := ProviderByID(ProviderFreeTSA)
	if !ok {
		t.Fatal("FreeTSA provider is missing")
	}
	profile.id = "synthetic-http"
	profile.endpoint = "http://tsa.example.test/stamp"
	profile.transport = providerTransportHTTP
	profile.sourceURL = "https://operator.example.test/ca.pem"
	profile.requestGuidance = "Synthetic test profile."
	transport := roundTripper(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != profile.Endpoint() {
			t.Fatalf("request URL = %s", req.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/timestamp-reply"}}, Body: io.NopCloser(bytes.NewReader(response))}, nil
	})
	result, err := (HTTPClient{Resolver: resolver, Client: &http.Client{Transport: transport}}).SubmitProvider(t.Context(), profile, request)
	if err != nil || result.TSAOrigin != "http://tsa.example.test:80" || result.RequestCount != 1 {
		t.Fatalf("SubmitProvider = %#v, %v", result, err)
	}
	redirect := roundTripper(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": []string{profile.Endpoint()}}, Body: http.NoBody, Request: req}, nil
	})
	if _, err := (HTTPClient{Resolver: resolver, Client: &http.Client{Transport: redirect}}).SubmitProvider(t.Context(), profile, request); err == nil {
		t.Fatal("built-in same-endpoint redirect succeeded")
	}
	if _, err := (HTTPClient{Resolver: resolver, Client: &http.Client{Transport: transport}}).Submit(t.Context(), profile.Endpoint(), request); SafeReason(err) != ReasonRequestProfile {
		t.Fatalf("custom HTTP = %v (%s)", err, SafeReason(err))
	}
}

func FuzzParseRequest(f *testing.F) {
	target, request, _, _ := fixture(f)
	f.Add(request, target)
	f.Add([]byte{0x30, 0}, target)
	f.Fuzz(func(t *testing.T, data, candidate []byte) {
		_, _ = ParseRequest(data, candidate, DefaultLimits())
	})
}

func FuzzParseResponse(f *testing.F) {
	_, _, response, _ := fixture(f)
	f.Add(response)
	f.Add([]byte{0x30, 0})
	f.Fuzz(func(t *testing.T, data []byte) {
		_ = ParseResponse(data, DefaultLimits())
	})
}

func FuzzVerifyResponse(f *testing.F) {
	target, request, response, root := fixture(f)
	f.Add(response)
	f.Add([]byte{0x30, 0})
	f.Fuzz(func(t *testing.T, candidate []byte) {
		if len(candidate) > DefaultLimits().ResponseBytes {
			return
		}
		_, _ = Verify(t.Context(), target, request, candidate, root, DefaultLimits())
	})
}

type staticResolver struct {
	addresses []net.IPAddr
	err       error
}

func (r staticResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return append([]net.IPAddr(nil), r.addresses...), r.err
}

type roundTripper func(*http.Request) (*http.Response, error)

func (fn roundTripper) RoundTrip(req *http.Request) (*http.Response, error) { return fn(req) }

type failingReadCloser struct{}

func (failingReadCloser) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func (failingReadCloser) Close() error             { return nil }

type testingTB interface {
	Helper()
	Fatal(...any)
}

func fixture(t testingTB) ([]byte, []byte, []byte, []byte) {
	return fixtureFrom(t, "target.txt", "request.tsq", "response.tsr", "root.pem")
}

func fixtureFrom(t testingTB, names ...string) ([]byte, []byte, []byte, []byte) {
	t.Helper()
	values := make([][]byte, len(names))
	for index, name := range names {
		data, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		values[index] = data
	}
	return values[0], values[1], values[2], values[3]
}

func fixtureFromDirectory(t testingTB, directory string, names ...string) ([]byte, []byte, []byte, []byte) {
	t.Helper()
	values := make([][]byte, len(names))
	for index, name := range names {
		data, err := os.ReadFile(filepath.Join("testdata", directory, name))
		if err != nil {
			t.Fatal(err)
		}
		values[index] = data
	}
	return values[0], values[1], values[2], values[3]
}
