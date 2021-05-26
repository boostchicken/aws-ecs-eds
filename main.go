package main

import (
	"context"
	"errors"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	endpointservice "github.com/envoyproxy/go-control-plane/envoy/service/endpoint/v3"
	"github.com/envoyproxy/go-control-plane/pkg/cache/types"
	"github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	gocache "github.com/patrickmn/go-cache"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"net"
	"os"
	"strconv"
	"time"
)

type server struct {
	ecs   *ecs.Client
	cache *gocache.Cache
}

func init() {

	// Log as JSON instead of the default ASCII formatter.
	log.SetFormatter(&log.TextFormatter{})

	// Output to stdout instead of the default stderr
	// Can be any io.Writer, see below for File example
	log.SetOutput(os.Stdout)

	// Only log the warning severity or above.
	log.SetLevel(log.InfoLevel)
}

func (*server) receive(stream endpointservice.EndpointDiscoveryService_StreamEndpointsServer, reqChannel chan *discovery.DiscoveryRequest) {
	for {
		req, err := stream.Recv()
		if err != nil {
			log.Error("Error while receiving message from stream", err)
			return
		}

		select {
		case reqChannel <- req:
		case <-stream.Context().Done():
			log.Error("Stream closed")
			return
		}
	}
}

func (s *server) StreamEndpoints(stream endpointservice.EndpointDiscoveryService_StreamEndpointsServer) error {
	stop := make(chan struct{})
	reqChannel := make(chan *discovery.DiscoveryRequest, 1)
	go s.receive(stream, reqChannel)

	for {
		select {
		case req, ok := <-reqChannel:
			if !ok {
				log.Error("Error receiving request")
				return errors.New("Error receiving request")
			}
			eds, cacheOk := s.cache.Get(req.ResourceNames[0])
			if !cacheOk {
				eds = s.generateEDS(req.ResourceNames[0])
				s.cache.Set(req.ResourceNames[0], eds, time.Minute*1)
			}
			response := cache.RawResponse{Version: req.VersionInfo,
				Resources: []types.ResourceWithTtl{{Resource: eds.(*endpoint.ClusterLoadAssignment)}},
				Request:   &discovery.DiscoveryRequest{TypeUrl: resource.EndpointType}}
			cacheResp, err := response.GetDiscoveryResponse()
			err = stream.Send(cacheResp)
			if err != nil {
				log.Error("Error StreamingEndpoint ", err)
				return err
			}
		case <-stop:
			return nil
		}
	}
}

func (s *server) DeltaEndpoints(stream endpointservice.EndpointDiscoveryService_DeltaEndpointsServer) error {
	log.Info("DeltaEndpoints service not implemented")
	return nil
}

func (*server) FetchEndpoints(ctx context.Context, req *discovery.DiscoveryRequest) (*discovery.DiscoveryResponse, error) {
	log.Info("FetchEndpoints service not implemented")
	return nil, nil
}

func (s *server) generateEDS(cluster string) *endpoint.ClusterLoadAssignment {
	var lbEndpoints = make([]*endpoint.LbEndpoint, 0)

	s.getTaskIps(&lbEndpoints, cluster, nil)

	ret := &endpoint.ClusterLoadAssignment{
		ClusterName: cluster,
		Endpoints: []*endpoint.LocalityLbEndpoints{
			{
				LbEndpoints: lbEndpoints,
			},
		},
	}

	return ret
}

func (s *server) getTaskIps(lbEndpoints *[]*endpoint.LbEndpoint, cluster string, nextToken *string) {
	taskArns, err := s.ecs.ListTasks(context.Background(), &ecs.ListTasksInput{Cluster: aws.String(cluster), NextToken: nextToken})
	if err != nil {
		log.Error("Error listing AWS tasks ", err)
		return
	}
	tasks, err := s.ecs.DescribeTasks(context.Background(), &ecs.DescribeTasksInput{
		Tasks: taskArns.TaskArns, Cluster: aws.String(cluster),
	})
	if err != nil {
		log.Error("Error Describing AWS tasks ", err)
		return
	}
	port, err := strconv.Atoi(os.Getenv(cluster + "_port"))
	if err != nil {
		port = 80
	}
	for _, task := range tasks.Tasks {
		for _, attachment := range task.Attachments {
			for _, details := range attachment.Details {
				if aws.ToString(details.Name) == "privateIPv4Address" {
					*lbEndpoints = append(*lbEndpoints, &endpoint.LbEndpoint{HostIdentifier: &endpoint.LbEndpoint_Endpoint{
						Endpoint: &endpoint.Endpoint{
							Address: &core.Address{
								Address: &core.Address_SocketAddress{
									SocketAddress: &core.SocketAddress{
										Address: aws.ToString(details.Value),
										PortSpecifier: &core.SocketAddress_PortValue{
											PortValue: uint32(port),
										},
									},
								},
							},
						},
					},
					})
				}
			}
		}
	}
	if taskArns.NextToken != nil {
		s.getTaskIps(lbEndpoints, cluster, taskArns.NextToken)
	}
}
func main() {
	grpcServer := grpc.NewServer()
	edsListen := os.Getenv("EDS_LISTEN")
	if edsListen == "" {
		edsListen = "0.0.0.0:5678"
	}
	lis, err := net.Listen("tcp", edsListen)
	if err != nil {
		log.Error(err)
	}

	cfg, _ := config.LoadDefaultConfig(context.Background())
	endpointservice.RegisterEndpointDiscoveryServiceServer(grpcServer, &server{ecs: ecs.NewFromConfig(cfg), cache: gocache.New(time.Minute*1, time.Minute*1)})

	reflection.Register(grpcServer)

	log.Infof("management server listening on %d", 5678)
	if err = grpcServer.Serve(lis); err != nil {
		log.Error(err)
	}
}
