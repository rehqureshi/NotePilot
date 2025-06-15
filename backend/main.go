package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
)

func transcribeWithDeepgram(filePath string, deepgramAPIKey string) (string, error) {
	url := "https://api.deepgram.com/v1/listen"

	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	// Detect content type based on file extension
	contentType := "audio/wav" // default

	ext := filepath.Ext(filePath)
	switch ext {
	case ".mp3":
		contentType = "audio/mpeg"
	case ".wav":
		contentType = "audio/wav"
	case ".flac":
		contentType = "audio/flac"
	}

	req, err := http.NewRequest("POST", url, file)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Token "+deepgramAPIKey)
	req.Header.Set("Content-Type", contentType)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("deepgram api error: %s", string(bodyBytes))
	}

	var responseData map[string]interface{}
	json.Unmarshal(bodyBytes, &responseData)

	transcript := responseData["results"].(map[string]interface{})["channels"].([]interface{})[0].(map[string]interface{})["alternatives"].([]interface{})[0].(map[string]interface{})["transcript"].(string)

	return transcript, nil
}

func summarizeWithTogether(transcript string, togetherAPIKey string) (string, error) {
	url := "https://api.together.xyz/v1/chat/completions"

	payload := map[string]interface{}{
		"model": "mistralai/Mixtral-8x7B-Instruct-v0.1",
		"messages": []map[string]string{
			{"role": "system", "content": "You are a meeting summarizer."},
			{"role": "user", "content": fmt.Sprintf("Summarize this meeting transcript:\n\n%s", transcript)},
		},
		"max_tokens":  500,
		"temperature": 0.5,
		"top_p":       1,
	}

	payloadBytes, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(payloadBytes))
	req.Header.Set("Authorization", "Bearer "+togetherAPIKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("together api error: %s", string(bodyBytes))
	}

	var responseData map[string]interface{}
	json.Unmarshal(bodyBytes, &responseData)

	choices := responseData["choices"].([]interface{})
	content := choices[0].(map[string]interface{})["message"].(map[string]interface{})["content"].(string)

	return content, nil
}

func main() {
	router := gin.Default()

	// Read environment variables once at startup
	deepgramAPIKey := os.Getenv("DEEPGRAM_API_KEY")
	togetherAPIKey := os.Getenv("TOGETHER_API_KEY")

	// Check if keys are missing (fail fast)
	if deepgramAPIKey == "" || togetherAPIKey == "" {
		fmt.Println("API keys are missing. Please set DEEPGRAM_API_KEY and TOGETHER_API_KEY environment variables.")
		os.Exit(1)
	}

	router.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	router.POST("/process", func(c *gin.Context) {
		file, err := c.FormFile("file")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "File is required"})
			return
		}

		audioDir := "audio"
		os.MkdirAll(audioDir, os.ModePerm)

		filename := filepath.Join(audioDir, "uploaded-"+file.Filename)
		if err := c.SaveUploadedFile(file, filename); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		fmt.Println("File saved:", filename)

		transcript, err := transcribeWithDeepgram(filename, deepgramAPIKey)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		summary, err := summarizeWithTogether(transcript, togetherAPIKey)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"summary": summary})
	})

	router.Run(":8080")
}
