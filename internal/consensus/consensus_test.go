package consensus

import (
	"context"
	"testing"
	"time"
)

func TestConsensusMode_Majority(t *testing.T) {
	responses := []DeliberationResponse{
		{ModelName: "ModelA", Response: "hello world", Confidence: 0.8},
		{ModelName: "ModelB", Response: "hello world", Confidence: 0.7},
		{ModelName: "ModelC", Response: "goodbye world", Confidence: 0.9},
	}

	winner := pickByMajority(responses)
	if winner == nil || winner.Response != "hello world" {
		t.Errorf("expected 'hello world', got %v", winner)
	}
}

func TestConsensusMode_Ranked(t *testing.T) {
	responses := []DeliberationResponse{
		{ModelName: "ModelA", Response: "response1", Confidence: 0.6},
		{ModelName: "ModelB", Response: "response2", Confidence: 0.9},
		{ModelName: "ModelC", Response: "response3", Confidence: 0.7},
	}

	winner := pickByRanked(responses)
	if winner == nil || winner.Confidence != 0.9 {
		t.Errorf("expected confidence 0.9, got %v", winner.Confidence)
	}
}

func TestConsensusMode_Weighted(t *testing.T) {
	responses := []DeliberationResponse{
		{ModelName: "ModelA", Response: "response1", Confidence: 0.8},
		{ModelName: "ModelB", Response: "response2", Confidence: 0.6},
	}

	winner := pickByWeighted(responses)
	if winner == nil {
		t.Error("expected non-nil winner")
	}
}

func TestStringSimilarity(t *testing.T) {
	tests := []struct {
		a, b     string
		expected float64
	}{
		{"hello world", "hello world", 1.0},
		{"hello world", "world hello", 1.0},
		{"hello", "goodbye", 0.0},
		{"hello world", "hello there", 0.5},
	}

	for _, tt := range tests {
		got := stringSimilarity(tt.a, tt.b)
		if got != tt.expected {
			t.Errorf("stringSimilarity(%q, %q) = %f, want %f", tt.a, tt.b, got, tt.expected)
		}
	}
}

func TestEstimateConfidence(t *testing.T) {
	tests := []struct {
		response string
		minConf  float64
		maxConf  float64
	}{
		{"Definitely correct answer", 0.55, 0.65},
		{"Maybe it could be right", 0.35, 0.45},
		{"Certainly the answer is clear", 0.60, 0.70},
	}

	for _, tt := range tests {
		conf := estimateConfidence(tt.response)
		if conf < tt.minConf || conf > tt.maxConf {
			t.Errorf("estimateConfidence(%q) = %f, want between %f and %f", tt.response, conf, tt.minConf, tt.maxConf)
		}
	}
}

func TestCalculateAgreement(t *testing.T) {
	responses := []DeliberationResponse{
		{ModelName: "ModelA", Response: "the same answer"},
		{ModelName: "ModelB", Response: "the same answer"},
		{ModelName: "ModelC", Response: "different response"},
	}

	agreement := calculateAgreement(responses)
	if agreement <= 0 || agreement > 1 {
		t.Errorf("agreement should be between 0 and 1, got %f", agreement)
	}
}

func TestEngine_Run_WithContext(t *testing.T) {
	endpoints := []ModelEndpoint{
		{Name: "Model1", URL: "http://localhost:8000", Tier: 2, Model: "gpt"},
	}

	engine := NewEngine(endpoints, ModeMajority)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := DeliberationRequest{
		Prompt:  "What is 2+2?",
		Models:  endpoints,
		Timeout: 2 * time.Second,
	}

	result, err := engine.Run(ctx, req)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result == nil {
		t.Error("expected non-nil result")
	}
}

func TestEngine_Run_EmptyEndpoints(t *testing.T) {
	engine := NewEngine([]ModelEndpoint{}, ModeMajority)
	ctx := context.Background()

	req := DeliberationRequest{
		Prompt: "What is 2+2?",
		Models: []ModelEndpoint{},
	}

	_, err := engine.Run(ctx, req)
	if err == nil {
		t.Error("expected error for empty endpoints")
	}
}

func TestFilterValidResponses(t *testing.T) {
	responses := []DeliberationResponse{
		{ModelName: "ModelA", Response: "valid", Error: ""},
		{ModelName: "ModelB", Response: "invalid", Error: "some error"},
		{ModelName: "ModelC", Response: "valid2", Error: ""},
	}

	valid := filterValidResponses(responses)
	if len(valid) != 2 {
		t.Errorf("expected 2 valid responses, got %d", len(valid))
	}
}

func TestPickByMajority_SingleResponse(t *testing.T) {
	responses := []DeliberationResponse{
		{ModelName: "ModelA", Response: "only one", Confidence: 0.5},
	}

	winner := pickByMajority(responses)
	if winner == nil || winner.Response != "only one" {
		t.Error("expected single response to be winner")
	}
}

func TestPickByRanked_EmptyResponses(t *testing.T) {
	responses := []DeliberationResponse{}
	winner := pickByRanked(responses)
	if winner != nil {
		t.Error("expected nil for empty responses")
	}
}

func TestConsensusResult_Message(t *testing.T) {
	endpoints := []ModelEndpoint{
		{Name: "Model1", URL: "http://localhost:8000", Tier: 2, Model: "gpt"},
	}

	engine := NewEngine(endpoints, ModeRanked)
	ctx := context.Background()

	req := DeliberationRequest{
		Prompt:  "Test?",
		Models:  endpoints,
		Timeout: 5 * time.Second,
	}

	result, err := engine.Run(ctx, req)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result == nil {
		t.Error("expected non-nil result")
	} else {
		if result.Strategy != "ranked" {
			t.Errorf("expected strategy 'ranked', got %s", result.Strategy)
		}
		if result.Message == "" {
			t.Error("expected non-empty message")
		}
	}
}
