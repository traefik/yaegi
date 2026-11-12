package awsplug

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// state uses the AWS SDK v2: SigV4 signing and presigning with a fixed
// time, an S3 client (PutObject, GetObject, ListObjectsV2 with its XML
// answers) against an in-memory S3 mock, and the DynamoDB attribute value
// marshaler (reflect on plugin types).
type state struct {
	srv    *httptest.Server
	s3     *s3.Client
	signer *v4.Signer
	creds  aws.Credentials

	mu      sync.Mutex
	objects map[string][]byte
}

// Item is marshaled to DynamoDB attribute values.
type Item struct {
	PK     string            `dynamodbav:"pk"`
	Count  int               `dynamodbav:"count"`
	Tags   []string          `dynamodbav:"tags,stringset"`
	Attrs  map[string]string `dynamodbav:"attrs,omitempty"`
	Hidden string            `dynamodbav:"-"`
}

func setup(ctx context.Context) (*state, error) {
	s := &state{objects: map[string][]byte{}, signer: v4.NewSigner()}
	s.srv = httptest.NewServer(http.HandlerFunc(s.mockS3))
	prov := credentials.NewStaticCredentialsProvider("AKIDEXAMPLE", "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY", "")
	creds, err := prov.Retrieve(ctx)
	if err != nil {
		return nil, err
	}
	s.creds = creds
	cfg := aws.Config{Region: "us-east-1", Credentials: prov, BaseEndpoint: aws.String(s.srv.URL), RetryMaxAttempts: 1}
	s.s3 = s3.NewFromConfig(cfg, func(o *s3.Options) { o.UsePathStyle = true })
	go func() {
		<-ctx.Done()
		s.srv.Close()
	}()
	return s, nil
}

func (s *state) process(req *http.Request) (string, error) {
	ctx := req.Context()
	// SigV4 of a copy of the request, at a fixed time.
	r2, _ := http.NewRequest(req.Method, "https://execute-api.us-east-1.amazonaws.com"+req.URL.RequestURI(), nil)
	sum := sha256.Sum256(nil)
	at := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	if err := s.signer.SignHTTP(ctx, s.creds, r2, hex.EncodeToString(sum[:]), "execute-api", "us-east-1", at); err != nil {
		return "", err
	}
	auth := r2.Header.Get("Authorization")
	sig := auth[strings.LastIndex(auth, "=")+1:]
	url, _, err := s.signer.PresignHTTP(ctx, s.creds, r2, hex.EncodeToString(sum[:]), "s3", "us-east-1", at)
	if err != nil {
		return "", err
	}

	// S3 through the mock.
	key := strings.Trim(req.URL.Path, "/")
	if key == "" {
		key = "index"
	}
	body := []byte(req.Method + " " + req.Host)
	if _, err := s.s3.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String("plugin"), Key: aws.String(key),
		Body: bytes.NewReader(body), ContentType: aws.String("text/plain"), Metadata: map[string]string{"from": "traefik"}}); err != nil {
		return "", err
	}
	got, err := s.s3.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String("plugin"), Key: aws.String(key)})
	if err != nil {
		return "", err
	}
	gb, _ := io.ReadAll(got.Body)
	got.Body.Close()
	list, err := s.s3.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String("plugin")})
	if err != nil {
		return "", err
	}
	var keys []string
	for _, o := range list.Contents {
		keys = append(keys, fmt.Sprintf("%s:%d", aws.ToString(o.Key), aws.ToInt64(o.Size)))
	}
	_, missErr := s.s3.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String("plugin"), Key: aws.String("missing")})

	// DynamoDB attribute values.
	av, err := attributevalue.MarshalMap(Item{PK: key, Count: len(keys), Tags: []string{"a", "b"}, Hidden: "x"})
	if err != nil {
		return "", err
	}
	var back Item
	if err := attributevalue.UnmarshalMap(av, &back); err != nil {
		return "", err
	}
	_, isN := av["count"].(*types.AttributeValueMemberN)
	names := make([]string, 0, len(av))
	for k := range av {
		names = append(names, k)
	}
	sort.Strings(names)

	return fmt.Sprintf("sig=%s presign=%t get=%q meta=%s list=%v miss=%t ddb=%v n=%t back=%s/%d/%v",
		sig[:16], strings.Contains(url, "X-Amz-Signature="), gb, got.Metadata["from"], keys, missErr != nil,
		names, isN, back.PK, back.Count, back.Tags), nil
}

// mockS3 is a path-style S3 with PUT, GET and ListObjectsV2.
func (s *state) mockS3(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/") {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	path := strings.TrimPrefix(r.URL.Path, "/plugin")
	switch {
	case r.Method == http.MethodPut:
		b, _ := io.ReadAll(r.Body)
		s.objects[strings.TrimPrefix(path, "/")] = b
		w.Header().Set("ETag", `"etag"`)
	case r.Method == http.MethodGet && r.URL.Query().Get("list-type") == "2":
		w.Header().Set("Content-Type", "application/xml")
		var ks []string
		for k := range s.objects {
			ks = append(ks, k)
		}
		sort.Strings(ks)
		fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?><ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Name>plugin</Name><KeyCount>`, len(ks), `</KeyCount><IsTruncated>false</IsTruncated>`)
		for _, k := range ks {
			fmt.Fprintf(w, `<Contents><Key>%s</Key><Size>%d</Size><StorageClass>STANDARD</StorageClass></Contents>`, k, len(s.objects[k]))
		}
		fmt.Fprint(w, `</ListBucketResult>`)
	case r.Method == http.MethodGet:
		b, ok := s.objects[strings.TrimPrefix(path, "/")]
		if !ok {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>NoSuchKey</Code><Message>no key</Message></Error>`)
			return
		}
		w.Header().Set("x-amz-meta-from", "traefik")
		w.Header().Set("Content-Type", "text/plain")
		w.Write(b)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
