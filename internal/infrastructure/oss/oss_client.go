package oss

import (
	"context"
	"fmt"
	"log"

	"control-panel/internal/config"
	"control-panel/pkg/oss"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func InitOSS(cfg *config.OSSConfig) (oss.OSSUploader, error) {
	if cfg.Endpoint == "" {
		// OSS disabled: return nil so callers can run without file uploads
		// (the server boots without object storage; uploads will no-op/error).
		return nil, nil
	}
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("OSS 未配置: bucket 不能为空，请设置 OSS_BUCKET 环境变量")
	}
	if cfg.Region == "" {
		return nil, fmt.Errorf("OSS 未配置: region 不能为空，请设置 OSS_REGION 环境变量")
	}

	ctx := context.Background()

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(cfg.Region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.AccessKey, cfg.SecretKey, "",
		)),
	)
	if err != nil {
		return nil, fmt.Errorf("加载 AWS 配置失败: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = &cfg.Endpoint
		o.UsePathStyle = cfg.ForcePathStyle

		// AWS SDK Go v2 defaults RequestChecksumCalculation to WhenSupported
		// (see config/resolve.go), which makes PutObject attach a default
		// CRC32 trailing checksum. The trailer is delivered via the
		// STREAMING-UNSIGNED-PAYLOAD-TRAILER payload-hash algorithm — an
		// AWS-only extension that Aliyun OSS, Tencent COS, MinIO and other
		// S3-compatible services reject with:
		//   400 NotImplemented: Aws MultiChunkedEncoding
		//   STREAMING-UNSIGNED-PAYLOAD-TRAILER is not supported.
		//
		// Downgrading to WhenRequired makes the SDK skip the default checksum
		// (the operation itself doesn't require one), which in turn skips the
		// trailer. The request body still goes through normal SigV4 payload
		// SHA256 signing, which is part of the S3 spec and works everywhere.
		//
		// Equivalent to setting env vars:
		//   AWS_REQUEST_CHECKSUM_CALCULATION=when_required
		//   AWS_RESPONSE_CHECKSUM_VALIDATION=when_required
		o.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
		o.ResponseChecksumValidation = aws.ResponseChecksumValidationWhenRequired
	})

	uploader := oss.NewS3Uploader(client, cfg.Bucket)
	log.Println("OSS 客户端初始化成功")
	return uploader, nil
}
