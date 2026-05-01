package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

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

	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	// TODO: implement the upload here
	const MaxMemory = 10 << 20 // 10 MB
	r.ParseMultipartForm(MaxMemory)

	file, header, err := r.FormFile("thumbnail")

	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err)
		return
	}
	defer file.Close()

	mediaType := header.Header.Get("Content-Type")

	// data, err := io.ReadAll(file)

	metadata, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "error getting metadata for by video", err)
		return
	}

	if metadata.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "You're not the owner of the video", err)
		return
	}

	// thumbnail := thumbnail{
	// 	data:      data,
	// 	mediaType: mediaType,
	// }

	// videoThumbnails[videoID] = thumbnail

	// var newURL string = fmt.Sprintf("/api/thumbnails/%s", videoID)
	// dataBase64 := base64.StdEncoding.EncodeToString(data)

	// var newURL string = fmt.Sprintf("data:%s;base64,%s", mediaType, dataBase64)
	fileExtension := ""
	switch mediaType {
	case "image/jpeg":
		fileExtension = "jpg"
	case "image/png":
		fileExtension = "png"
	default:
		respondWithError(w, http.StatusBadRequest, "Unsupported media type", nil)
		return
	}
	var newPath string = filepath.Join(cfg.assetsRoot, fmt.Sprintf("%s.%s", videoID, fileExtension))
	newFile, err := os.Create(newPath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error creating the thumbnail file", err)
		return
	}
	defer newFile.Close()

	var newURL string = fmt.Sprintf("http://localhost:8091/%s/%s.%s", cfg.assetsRoot, videoID, fileExtension)
	fmt.Printf("new thumbnail URL: %s\n", newURL)
	metadata.ThumbnailURL = &newURL

	_, err = io.Copy(newFile, file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error writing the thumbnail file", err)
		return
	}

	err = cfg.db.UpdateVideo(metadata)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error saving the new thumbnail", err)
		return
	}

	newVideoJson, err := json.Marshal(metadata)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error with marshaling the video info", err)
		return
	}
	respondWithJSON(w, http.StatusOK, newVideoJson)
}
