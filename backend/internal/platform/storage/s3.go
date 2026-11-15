package storage

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3 stores media in a private bucket with SSE. Uses the Lambda IAM role.
type S3 struct {
	bucket  string
	client  *s3.Client
	presign *s3.PresignClient
}

// NewS3 builds the S3 storage.
func NewS3(cfg aws.Config, bucket string) *S3 {
	c := s3.NewFromConfig(cfg)
	return &S3{bucket: bucket, client: c, presign: s3.NewPresignClient(c)}
}

func (s *S3) PresignPut(ctx context.Context, key, contentType string, size int64, ttl time.Duration) (string, map[string]string, error) {
	req, err := s.presign.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:               aws.String(s.bucket),
		Key:                  aws.String(key),
		ContentType:          aws.String(contentType),
		ContentLength:        aws.Int64(size),
		ServerSideEncryption: types.ServerSideEncryptionAes256,
	}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", nil, errors.New("storage: presign put failed")
	}
	return req.URL, map[string]string{
		"Content-Type":                 contentType,
		"Content-Length":               strconv.FormatInt(size, 10),
		"x-amz-server-side-encryption": "AES256",
	}, nil
}

func (s *S3) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	req, err := s.presign.PresignGetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", errors.New("storage: presign get failed")
	}
	return req.URL, nil
}

func (s *S3) Head(ctx context.Context, key string) (Object, error) {
	out, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	if err != nil {
		var nf *types.NotFound
		if errors.As(err, &nf) {
			return Object{}, ErrNotFound
		}
		return Object{}, errors.New("storage: head failed")
	}
	return Object{Size: aws.ToInt64(out.ContentLength), ContentType: aws.ToString(out.ContentType)}, nil
}

func (s *S3) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	if err != nil {
		return errors.New("storage: delete failed")
	}
	return nil
}
