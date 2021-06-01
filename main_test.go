package main

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/servicediscovery"
	gocache "github.com/patrickmn/go-cache"
	"testing"
)

func TestGenerateEds(t *testing.T) {
	cfg, _ := config.LoadDefaultConfig(context.Background(), config.WithRegion("us-east-1"))
	s := &server{ecs: ecs.NewFromConfig(cfg), servicediscovery: servicediscovery.NewFromConfig(cfg), cache: gocache.New(0, 0)}
	ret := s.generateEDS("srv-qp3a4lugw4s5ei3a")
	t.Log(ret)
}
