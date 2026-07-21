package storage

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"go.uber.org/zap"
)

// S3Config is persisted in SQLite through an S3 connection. It intentionally
// does not belong to the process config file.
type S3Config struct {
	Bucket, Region, Endpoint, PublicBaseURL, Prefix string
	ForcePathStyle                                  bool
	AccessKeyID, SecretAccessKey                    string
}

type S3Store struct {
	client         *s3.Client
	presignClient  *s3.PresignClient
	bucket         string
	region         string
	endpoint       string
	prefix         string
	forcePathStyle bool
	publicEndpoint string
	log            *zap.Logger
}

func (s *S3Store) Read(ctx context.Context, object Object) ([]byte, error) {
	key := normalizeObjectKey(object.Key)
	if key == "" {
		return nil, ErrRemoteURLUnavailable
	}
	result, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(stringsTrimFallback(object.Bucket, s.bucket)),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("s3 get object %s: %w", key, err)
	}
	defer result.Body.Close()
	data, err := io.ReadAll(io.LimitReader(result.Body, (512<<20)+1))
	if err != nil {
		return nil, fmt.Errorf("read s3 object %s: %w", key, err)
	}
	if len(data) > 512<<20 {
		return nil, fmt.Errorf("s3 object %s exceeds 512 MB download limit", key)
	}
	return data, nil
}

func (s *S3Store) Delete(ctx context.Context, object Object) error {
	key := normalizeObjectKey(object.Key)
	if key == "" {
		return nil
	}
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(stringsTrimFallback(object.Bucket, s.bucket)),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("s3 delete object %s: %w", key, err)
	}
	return nil
}

func NewS3Store(cfg S3Config, log *zap.Logger) (*S3Store, error) {
	if log == nil {
		log = zap.NewNop()
	}
	endpoint := strings.TrimRight(strings.TrimSpace(cfg.Endpoint), "/")
	awsCfg, err := awsconfig.LoadDefaultConfig(
		context.Background(),
		awsconfig.WithRegion(strings.TrimSpace(cfg.Region)),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			strings.TrimSpace(cfg.AccessKeyID),
			strings.TrimSpace(cfg.SecretAccessKey),
			"",
		)),
	)
	if err != nil {
		return nil, fmt.Errorf("load s3 config: %w", err)
	}
	client := s3.NewFromConfig(awsCfg, func(options *s3.Options) {
		if endpoint != "" {
			options.BaseEndpoint = aws.String(endpoint)
		}
		options.UsePathStyle = cfg.ForcePathStyle
	})
	publicEndpoint := strings.TrimRight(strings.TrimSpace(cfg.PublicBaseURL), "/")
	presignClient := client
	if publicEndpoint != "" && publicEndpoint != endpoint {
		presignClient = s3.NewFromConfig(awsCfg, func(options *s3.Options) {
			options.BaseEndpoint = aws.String(publicEndpoint)
			options.UsePathStyle = cfg.ForcePathStyle
		})
	}
	return &S3Store{
		client:         client,
		presignClient:  s3.NewPresignClient(presignClient),
		bucket:         strings.TrimSpace(cfg.Bucket),
		region:         strings.TrimSpace(cfg.Region),
		endpoint:       endpoint,
		prefix:         normalizeObjectKey(cfg.Prefix),
		forcePathStyle: cfg.ForcePathStyle,
		publicEndpoint: publicEndpoint,
		log:            log,
	}, nil
}

func (s *S3Store) Backend() string {
	return BackendS3
}

func (s *S3Store) Upload(ctx context.Context, input UploadInput) (Object, error) {
	key := s.objectKey(input.Key)
	body, size, err := input.body()
	if err != nil {
		return Object{}, err
	}
	contentType := strings.TrimSpace(input.ContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if _, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(key),
		Body:          body,
		ContentLength: aws.Int64(size),
		ContentType:   aws.String(contentType),
	}); err != nil {
		return Object{}, fmt.Errorf("s3 put object %s: %w", key, err)
	}
	object := Object{
		Backend: BackendS3,
		Bucket:  s.bucket,
		Key:     key,
		URL:     s.objectURL(key),
	}
	s.log.Info("s3 object uploaded",
		zap.String("bucket", object.Bucket),
		zap.String("key", object.Key),
		zap.Int64("bytes", size),
	)
	return object, nil
}

func (s *S3Store) PresignGet(ctx context.Context, object Object, ttl time.Duration) (SignedURL, error) {
	key := normalizeObjectKey(object.Key)
	if key == "" {
		return SignedURL{}, ErrRemoteURLUnavailable
	}
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	result, err := s.presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(stringsTrimFallback(object.Bucket, s.bucket)),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(ttl))
	if err != nil {
		return SignedURL{}, fmt.Errorf("s3 presign object %s: %w", key, err)
	}
	return SignedURL{
		URL:       result.URL,
		ExpiresAt: time.Now().UTC().Add(ttl),
	}, nil
}

func (s *S3Store) objectKey(key string) string {
	key = normalizeObjectKey(key)
	if s.prefix == "" {
		return key
	}
	if key == "" {
		return s.prefix
	}
	if strings.HasPrefix(key, s.prefix+"/") {
		return key
	}
	return s.prefix + "/" + key
}

func (s *S3Store) objectURL(key string) string {
	escapedKey := strings.ReplaceAll(url.PathEscape(key), "%2F", "/")
	if s.publicEndpoint != "" {
		return s.endpointObjectURLAt(s.publicEndpoint, escapedKey)
	}
	if s.endpoint != "" {
		return s.endpointObjectURL(escapedKey)
	}
	if s.region == "" || s.bucket == "" {
		return ""
	}
	return fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", s.bucket, s.region, escapedKey)
}

func (s *S3Store) endpointObjectURL(escapedKey string) string {
	return s.endpointObjectURLAt(s.endpoint, escapedKey)
}

func (s *S3Store) endpointObjectURLAt(endpoint, escapedKey string) string {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" {
		return endpoint + "/" + escapedKey
	}
	if s.forcePathStyle {
		return strings.TrimRight(endpoint, "/") + "/" + url.PathEscape(s.bucket) + "/" + escapedKey
	}
	if strings.HasPrefix(parsed.Host, s.bucket+".") {
		return strings.TrimRight(endpoint, "/") + "/" + escapedKey
	}
	parsed.Host = s.bucket + "." + parsed.Host
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/" + escapedKey
	parsed.RawQuery = ""
	return parsed.String()
}

func stringsTrimFallback(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
