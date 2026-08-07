package oss

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type S3Uploader struct {
	client *s3.Client
	bucket string
}

func NewS3Uploader(client *s3.Client, bucket string) *S3Uploader {
	return &S3Uploader{client: client, bucket: bucket}
}

func (u *S3Uploader) Upload(ctx context.Context, key string, reader io.Reader, size int64) (string, error) {
	buf := new(bytes.Buffer)
	teeReader := io.TeeReader(reader, buf)

	hash := sha256.New()
	if _, err := io.Copy(hash, teeReader); err != nil {
		return "", fmt.Errorf("读取文件内容失败: %w", err)
	}

	fileHash := fmt.Sprintf("%x", hash.Sum(nil))

	_, err := u.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(u.bucket),
		Key:           aws.String(key),
		Body:          bytes.NewReader(buf.Bytes()),
		ContentLength: aws.Int64(size),
	})
	if err != nil {
		return "", fmt.Errorf("上传至 OSS 失败: %w", err)
	}

	return fileHash, nil
}

func (u *S3Uploader) GetPresignedURL(ctx context.Context, key string) (string, error) {
	presignClient := s3.NewPresignClient(u.client)

	request, err := presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(u.bucket),
		Key:    aws.String(key),
	}, func(opts *s3.PresignOptions) {
		opts.Expires = 1 * time.Hour
	})
	if err != nil {
		return "", fmt.Errorf("生成预签名 URL 失败: %w", err)
	}

	return request.URL, nil
}

func (u *S3Uploader) Delete(ctx context.Context, key string) error {
	_, err := u.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(u.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("删除 OSS 文件失败: %w", err)
	}

	return nil
}

// Download fetches an object from S3 and returns its body as a ReadCloser.
// The caller is responsible for closing the returned reader.
func (u *S3Uploader) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	out, err := u.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(u.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("从 OSS 下载文件失败: %w", err)
	}
	return out.Body, nil
}
