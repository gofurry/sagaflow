package storage

import (
	"context"
	"net/url"
	"testing"
	"time"
)

func TestS3StoreBuildsTencentCOSObjectURL(t *testing.T) {
	store, err := NewS3Store(S3Config{
		Bucket:          "dev-1307743912",
		Region:          "ap-chengdu",
		Endpoint:        "https://cos.ap-chengdu.myqcloud.com",
		AccessKeyID:     "test-id",
		SecretAccessKey: "test-key",
		Prefix:          "sagaflow/dev",
		ForcePathStyle:  false,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	key := store.objectKey("assets/videos/video.mp4")
	if key != "sagaflow/dev/assets/videos/video.mp4" {
		t.Fatalf("unexpected object key: %s", key)
	}
	got := store.objectURL(key)
	want := "https://dev-1307743912.cos.ap-chengdu.myqcloud.com/sagaflow/dev/assets/videos/video.mp4"
	if got != want {
		t.Fatalf("unexpected object url:\n got: %s\nwant: %s", got, want)
	}
}

func TestS3StoreBuildsPathStyleObjectURL(t *testing.T) {
	store, err := NewS3Store(S3Config{
		Bucket:          "sagaflow",
		Region:          "us-east-1",
		Endpoint:        "http://localhost:9000",
		AccessKeyID:     "test-id",
		SecretAccessKey: "test-key",
		Prefix:          "dev",
		ForcePathStyle:  true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	got := store.objectURL(store.objectKey("assets/image.png"))
	want := "http://localhost:9000/sagaflow/dev/assets/image.png"
	if got != want {
		t.Fatalf("unexpected object url:\n got: %s\nwant: %s", got, want)
	}
}

func TestS3StoreUsesPublicEndpointForSignedURLs(t *testing.T) {
	store, err := NewS3Store(S3Config{
		Bucket:          "sagaflow",
		Region:          "us-east-1",
		Endpoint:        "http://minio.internal:9000",
		PublicBaseURL:   "https://objects.example.com",
		AccessKeyID:     "test-id",
		SecretAccessKey: "test-key",
		ForcePathStyle:  true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	signed, err := store.PresignGet(context.Background(), Object{Bucket: "sagaflow", Key: "assets/image.png"}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(signed.URL)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Host != "objects.example.com" {
		t.Fatalf("signed URL used the internal endpoint: %s", signed.URL)
	}
	if parsed.Path != "/sagaflow/assets/image.png" {
		t.Fatalf("unexpected signed URL path: %s", parsed.Path)
	}
}
