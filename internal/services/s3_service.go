package services

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"path/filepath"
	"time"
	"whatsapp-bot/config"
	"whatsapp-bot/internal/utils"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

type S3Service struct {
	s3Client *s3.S3
	config   *config.S3Config
}

func NewS3Service(config *config.S3Config) (*S3Service, error) {
	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String("us-east-1"),
		Credentials: credentials.NewStaticCredentials(config.AccessKey, config.SecretKey, ""),
		Endpoint:    aws.String(config.ServiceUrl),
	})
	if err != nil {
		return nil, fmt.Errorf("erro ao criar sessão do S3: %v", err)
	}

	return &S3Service{
		s3Client: s3.New(sess),
		config:   config,
	}, nil
}

func (s *S3Service) UploadFile(file multipart.File, fileHeader *multipart.FileHeader) (string, error) {
	buffer := make([]byte, fileHeader.Size)
	if _, err := file.Read(buffer); err != nil {
		return "", fmt.Errorf("erro ao ler arquivo: %v", err)
	}

	// Voltar ao início do arquivo para upload
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("erro ao resetar arquivo: %v", err)
	}

	// Gerar nome único para o arquivo
	ext := filepath.Ext(fileHeader.Filename)
	filename := fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)

	// Detectar o tipo MIME do arquivo
	contentType := fileHeader.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	utils.LogInfo("Iniciando upload para S3: %s", filename)

	// Parâmetros para upload no S3
	params := &s3.PutObjectInput{
		Bucket:      aws.String(s.config.BucketName),
		Key:         aws.String(filename),
		Body:        bytes.NewReader(buffer),
		ContentType: aws.String(contentType),
	}

	// Realizar upload
	_, err := s.s3Client.PutObject(params)
	if err != nil {
		return "", fmt.Errorf("erro ao fazer upload para S3: %v", err)
	}

	// Retornar URL do arquivo
	fileUrl := fmt.Sprintf("%s/%s", s.config.BucketUrl, filename)
	utils.LogInfo("Upload concluído: %s", fileUrl)

	return fileUrl, nil
}

func (s *S3Service) UploadBytes(data []byte, fileName string, contentType string) (string, error) {
	params := &s3.PutObjectInput{
		Bucket:      aws.String(s.config.BucketName),
		Key:         aws.String(fileName),
		Body:        bytes.NewReader(data),
		ContentType: aws.String(contentType),
	}

	_, err := s.s3Client.PutObject(params)
	if err != nil {
		return "", fmt.Errorf("erro ao fazer upload para S3: %v", err)
	}

	fileUrl := fmt.Sprintf("%s/%s", s.config.BucketUrl, fileName)
	return fileUrl, nil
}
