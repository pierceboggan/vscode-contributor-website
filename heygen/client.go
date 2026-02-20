package heygen

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

const (
	baseURL = "https://api.heygen.com"
	// Default avatar and voice - can be overridden via env vars
	defaultAvatarID = "Abigail_expressive_2024112501"
	defaultVoiceID  = "1bd001e7e50f421d891986aad5158bc8" // Sara - friendly female voice
)

// Milestones that trigger celebration videos
var Milestones = []int{5, 10, 25, 50, 100, 250, 500, 1000}

// IsMilestone checks if a PR count is a celebration milestone
func IsMilestone(prCount int) bool {
	for _, m := range Milestones {
		if prCount == m {
			return true
		}
	}
	return false
}

// Client handles HeyGen API communication
type Client struct {
	apiKey     string
	avatarID   string
	voiceID    string
	httpClient *http.Client
}

// NewClient creates a new HeyGen API client
func NewClient() *Client {
	avatarID := os.Getenv("HEYGEN_AVATAR_ID")
	if avatarID == "" {
		avatarID = defaultAvatarID
	}
	voiceID := os.Getenv("HEYGEN_VOICE_ID")
	if voiceID == "" {
		voiceID = defaultVoiceID
	}

	return &Client{
		apiKey:     os.Getenv("HEYGEN_API_KEY"),
		avatarID:   avatarID,
		voiceID:    voiceID,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// IsConfigured returns true if API credentials are set
func (c *Client) IsConfigured() bool {
	return c.apiKey != ""
}

// GenerateVideoRequest represents the request to generate a video
type GenerateVideoRequest struct {
	ContributorName string
	GitHubUsername  string
	Milestone       int
}

// GenerateVideoResponse contains the video generation result
type GenerateVideoResponse struct {
	VideoID string `json:"video_id"`
	Status  string `json:"status"`
}

// VideoStatusResponse contains video status and URL when complete
type VideoStatusResponse struct {
	Status   string `json:"status"`
	VideoURL string `json:"video_url,omitempty"`
	Error    string `json:"error,omitempty"`
}

// GenerateVideo creates a celebration video dynamically
func (c *Client) GenerateVideo(req GenerateVideoRequest) (*GenerateVideoResponse, error) {
	if !c.IsConfigured() {
		return nil, fmt.Errorf("HeyGen API not configured")
	}

	// Build the celebration script
	script := fmt.Sprintf(
		"Congratulations %s! You've just hit an incredible milestone — %d Pull Requests merged into VS Code! "+
			"Your contributions are shaping the editor used by millions of developers worldwide. "+
			"Thank you for being part of the VS Code community. Here's to many more!",
		req.ContributorName, req.Milestone,
	)

	payload := map[string]interface{}{
		"video_inputs": []map[string]interface{}{
			{
				"character": map[string]interface{}{
					"type":      "avatar",
					"avatar_id": c.avatarID,
					"avatar_style": "normal",
				},
				"voice": map[string]interface{}{
					"type":     "text",
					"input_text": script,
					"voice_id": c.voiceID,
				},
			},
		},
		"dimension": map[string]interface{}{
			"width":  1280,
			"height": 720,
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/v2/video/generate", baseURL)
	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("X-Api-Key", c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		var errResp map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&errResp)
		return nil, fmt.Errorf("HeyGen API returned status %d: %v", resp.StatusCode, errResp)
	}

	var result struct {
		Data struct {
			VideoID string `json:"video_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &GenerateVideoResponse{
		VideoID: result.Data.VideoID,
		Status:  "pending",
	}, nil
}

// GetVideoStatus checks the status of a video generation
func (c *Client) GetVideoStatus(videoID string) (*VideoStatusResponse, error) {
	if !c.IsConfigured() {
		return nil, fmt.Errorf("HeyGen API not configured")
	}

	url := fmt.Sprintf("%s/v1/video_status.get?video_id=%s", baseURL, videoID)
	httpReq, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("X-Api-Key", c.apiKey)
	httpReq.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HeyGen API returned status %d", resp.StatusCode)
	}

	var result struct {
		Data struct {
			Status   string `json:"status"`
			VideoURL string `json:"video_url"`
			Error    string `json:"error"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &VideoStatusResponse{
		Status:   result.Data.Status,
		VideoURL: result.Data.VideoURL,
		Error:    result.Data.Error,
	}, nil
}
