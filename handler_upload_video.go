package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"os"
	"os/exec"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	maxLimitVideoSize := int64(1 << 30) // 1 GB
	r.Body = http.MaxBytesReader(w, r.Body, maxLimitVideoSize)

	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	videoMetadata, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't retrieve video metadata", err)
		return
	}

	if videoMetadata.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "You don't have permission to upload a video for this video ID", nil)
		return
	}

	file, fileHeader, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't read video file", err)
		return
	}
	defer file.Close()

	mediaType, _, err := mime.ParseMediaType(fileHeader.Header.Get("Content-Type"))

	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't parse media type", err)
		return
	}

	if mediaType != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "Only MP4 videos are allowed", nil)
		return
	}

	tempFile, err := os.CreateTemp("", "tubely-upload-*.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't create temporary file", err)
		return
	}
	defer tempFile.Close()
	defer os.Remove(tempFile.Name())
	io.Copy(tempFile, file)

	file.Seek(0, io.SeekStart)

	aspectRatio, err := getVideoAspectRatio(tempFile.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't get video aspect ratio", err)
		return
	}
	prefixVideoName := "other"
	switch aspectRatio {
	case "16:9":
		prefixVideoName = "landscape"
	case "9:16":
		prefixVideoName = "portrait"
	}

	var videoFileKeyRawData [32]byte
	rand.Read(videoFileKeyRawData[:])
	videoFileKey := hex.EncodeToString(videoFileKeyRawData[:])

	videoFileKey = prefixVideoName + "/" + videoFileKey + ".mp4"

	cfg.s3Client.PutObject(r.Context(), &s3.PutObjectInput{
		Bucket:      &cfg.s3Bucket,
		Key:         &videoFileKey,
		Body:        file,
		ContentType: &mediaType,
	})

	videoURL := "https://" + cfg.s3Bucket + ".s3." + cfg.s3Region + ".amazonaws.com/" + videoFileKey

	videoMetadata.VideoURL = &videoURL
	err = cfg.db.UpdateVideo(videoMetadata)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update video URL in database", err)
		return
	}

	w.WriteHeader(http.StatusOK)

}

func getVideoAspectRatio(filePath string) (string, error) {
	result := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)
	var out bytes.Buffer
	result.Stdout = &out
	err := result.Run()
	if err != nil {
		return "", err
	}

	type ffprobeOutput struct {
		Streams []struct {
			AspectRatio string `json:"display_aspect_ratio"`
		} `json:"streams"`
	}

	var ffprobeData ffprobeOutput
	err = json.Unmarshal(out.Bytes(), &ffprobeData)
	if err != nil {
		return "", err
	}

	if len(ffprobeData.Streams) == 0 {
		return "", nil
	}

	knowAspectRatios := []string{"16:9", "9:16"}
	currentAspectRatio := ffprobeData.Streams[0].AspectRatio

	for _, aspectRatio := range knowAspectRatios {
		if aspectRatio == currentAspectRatio {
			return aspectRatio, nil
		}
	}

	return "other", nil
}
