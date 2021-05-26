package main

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"testing"
)

func TestGenerateEds(t *testing.T) {
	cfg, _ := config.LoadDefaultConfig(context.Background(), config.WithRegion("us-east-1"))
	s := &server{ecs: ecs.NewFromConfig(cfg)}
	ret := s.generateEDS("lootlink-web")
	t.Log(ret)
}
