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

const (
	deepgramAPIKey = "04b656c73828bb69731b0078f11cd9a7df20ebe5"
	togetherAPIKey = "83f6b59a09f20da69086b3e73194d02796cf79bc6db9d3d2eed1ef03762f3241"
)

func transcribeWithDeepgram(filePath string) (string, error) {
	url := "https://api.deepgram.com/v1/listen"

	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	req, err := http.NewRequest("POST", url, file)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Token "+deepgramAPIKey)
	req.Header.Set("Content-Type", "audio/wav") // Adjust this if you're uploading mp3 etc.

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("Deepgram API error: %s", string(bodyBytes))
	}

	var responseData map[string]interface{}
	json.Unmarshal(bodyBytes, &responseData)

	transcript := responseData["results"].(map[string]interface{})["channels"].([]interface{})[0].(map[string]interface{})["alternatives"].([]interface{})[0].(map[string]interface{})["transcript"].(string)

	return transcript, nil
}

func summarizeWithTogether(transcript string) (string, error) {
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
		return "", fmt.Errorf("Together AI error: %s", string(bodyBytes))
	}

	var responseData map[string]interface{}
	json.Unmarshal(bodyBytes, &responseData)

	choices := responseData["choices"].([]interface{})
	content := choices[0].(map[string]interface{})["message"].(map[string]interface{})["content"].(string)

	return content, nil
}

func main() {
	router := gin.Default()

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
		os.MkdirAll(audioDir, os.ModePerm) // ensures folder exists

		filename := filepath.Join(audioDir, "uploaded-"+file.Filename)
		if err := c.SaveUploadedFile(file, filename); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		fmt.Println("File saved:", filename)

		// Transcribe using Deepgram
		transcript, err := transcribeWithDeepgram(filename)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		// Summarize using Together AI
		summary, err := summarizeWithTogether(transcript)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"summary": summary})
	})

	router.Run(":8080")
}
