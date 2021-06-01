package main

import (
	"context"
	"errors"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/servicediscovery"
	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	endpointservice "github.com/envoyproxy/go-control-plane/envoy/service/endpoint/v3"
	"github.com/envoyproxy/go-control-plane/pkg/cache/types"
	"github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	gocache "github.com/patrickmn/go-cache"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var srv *server

type server struct {
	ecs              *ecs.Client
	servicediscovery *servicediscovery.Client
	cache            *gocache.Cache
}

func init() {
	cfg, _ := config.LoadDefaultConfig(context.Background())
	srv = &server{ecs: ecs.NewFromConfig(cfg), servicediscovery: servicediscovery.NewFromConfig(cfg), cache: gocache.New(time.Second*30, time.Second*30)}
	log.SetFormatter(&log.TextFormatter{})
	log.SetOutput(os.Stdout)
	log.SetLevel(log.InfoLevel)
}

func (*server) receive(stream endpointservice.EndpointDiscoveryService_StreamEndpointsServer, reqChannel chan *discovery.DiscoveryRequest) {
	for {
		req, err := stream.Recv()
		if err != nil {
			log.Debug("error while receiving message from stream: ", err)
			return
		}

		select {
		case reqChannel <- req:
		case <-stream.Context().Done():
			log.Debug("Stream closed")
			return
		}
	}
}

func (s *server) StreamEndpoints(stream endpointservice.EndpointDiscoveryService_StreamEndpointsServer) error {

	reqChannel := make(chan *discovery.DiscoveryRequest, 1)
	go s.receive(stream, reqChannel)

	for {
		select {
		case req, ok := <-reqChannel:
			if !ok {
				log.Error("error receiving request")
				return errors.New("error receiving request")
			}
			cacheResp, cacheOk := s.cache.Get(req.ResourceNames[0])
			if !cacheOk {
				eds := s.generateEDS(req.ResourceNames[0])
				response := cache.RawResponse{Version: strconv.FormatInt(time.Now().Unix(), 10),
					Resources: []types.ResourceWithTtl{{Resource: eds}},
					Request:   req}
				cacheResp, _ = response.GetDiscoveryResponse()

				s.cache.Set(req.ResourceNames[0], cacheResp, time.Second*30)
			}
			err := stream.Send(cacheResp.(*discovery.DiscoveryResponse))
			if err != nil {
				log.Error("StreamingEndpoint-Send", err)
				return err
			}
		}
	}
}

func (s *server) DeltaEndpoints(stream endpointservice.EndpointDiscoveryService_DeltaEndpointsServer) error {
	log.Info("DeltaEndpoints service not implemented")
	return nil
}

func (s *server) FetchEndpoints(ctx context.Context, req *discovery.DiscoveryRequest) (*discovery.DiscoveryResponse, error) {
	var err error
	cacheResp, cacheOk := s.cache.Get(req.ResourceNames[0])
	if !cacheOk {
		eds := s.generateEDS(req.ResourceNames[0])
		s.cache.Set(req.ResourceNames[0], eds, time.Second*30)
		response := cache.RawResponse{Version: strconv.FormatInt(time.Now().Unix(), 10),
			Resources: []types.ResourceWithTtl{{Resource: eds}},
			Request:   req}
		cacheResp, err = response.GetDiscoveryResponse()
		s.cache.Set(req.ResourceNames[0], cacheResp, time.Minute*1)
	}
	return cacheResp.(*discovery.DiscoveryResponse), err
}

func (s *server) generateEDS(cluster string) *endpoint.ClusterLoadAssignment {

	var lbEndpoints = make([]*endpoint.LbEndpoint, 0)
	var endpointsChan = make(chan *endpoint.LbEndpoint, 1)

	if strings.Contains(cluster, "srv-") {
		log.Info("Generating new EDS values - Cloudmap")
		go s.getServiceDiscoveryIps(endpointsChan, cluster)
	} else {
		log.Info("Generating new EDS values - ECS")
		go s.getTaskIps(endpointsChan, cluster)
	}

	for i := range endpointsChan {
		lbEndpoints = append(lbEndpoints, i)
	}

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

func (s *server) getTaskIps(lbEndpoints chan *endpoint.LbEndpoint, cluster string) {
	listTasks := ecs.NewListTasksPaginator(s.ecs, &ecs.ListTasksInput{Cluster: aws.String(cluster)})
	for listTasks.HasMorePages() {
		taskArns, err := listTasks.NextPage(context.TODO())
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
						lbEndpoints <- &endpoint.LbEndpoint{HostIdentifier: &endpoint.LbEndpoint_Endpoint{
							Endpoint: &endpoint.Endpoint{
								Address: &core.Address{
									Address: &core.Address_SocketAddress{
										SocketAddress: &core.SocketAddress{
											Protocol: core.SocketAddress_TCP,
											Address:  aws.ToString(details.Value),
											PortSpecifier: &core.SocketAddress_PortValue{
												PortValue: uint32(port),
											},
										},
									},
								},
							},
						},
						}
					}
				}
			}
		}
	}
	close(lbEndpoints)
}

func (s *server) getServiceDiscoveryIps(lbEndpoints chan *endpoint.LbEndpoint, serviceId string) {
	listInstances := servicediscovery.NewListInstancesPaginator(s.servicediscovery, &servicediscovery.ListInstancesInput{ServiceId: aws.String(serviceId)})
	for listInstances.HasMorePages() {
		instances, err := listInstances.NextPage(context.TODO())
		if err != nil {
			log.Error(err)
		}
		for _, instance := range instances.Instances {
			port, err2 := strconv.Atoi(os.Getenv(serviceId + "_port"))
			if err2 != nil {
				port, err2 = strconv.Atoi(instance.Attributes["AWS_INSTANCE_PORT"])
				if err2 != nil {
					port = 80
				}
			}
			lbEndpoints <- &endpoint.LbEndpoint{HostIdentifier: &endpoint.LbEndpoint_Endpoint{
				Endpoint: &endpoint.Endpoint{
					Address: &core.Address{
						Address: &core.Address_SocketAddress{
							SocketAddress: &core.SocketAddress{
								Protocol: core.SocketAddress_TCP,
								Address:  instance.Attributes["AWS_INSTANCE_IPV4"],
								PortSpecifier: &core.SocketAddress_PortValue{
									PortValue: uint32(port),
								},
							},
						},
					},
				},
			},
			}

		}
	}
	close(lbEndpoints)
}

func main() {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGKILL, syscall.SIGINT, syscall.SIGTERM)

	grpcServer := grpc.NewServer()

	edsListen := os.Getenv("EDS_LISTEN")
	if edsListen == "" {
		edsListen = "0.0.0.0:5678"
	}

	lis, err := net.Listen("tcp", edsListen)
	if err != nil {
		log.Error(err)
		os.Exit(-2)
	}

	go func() {
		endpointservice.RegisterEndpointDiscoveryServiceServer(grpcServer, srv)

		reflection.Register(grpcServer)

		log.Infof("management server listening on %s", edsListen)
		if err = grpcServer.Serve(lis); err != nil {
			log.Error(err)
			os.Exit(-1)
		}
	}()

	sig := <-sigs
	log.Printf("Caught Signal %v", sig)
	go grpcServer.GracefulStop()
	time.Sleep(time.Second * 5)
	grpcServer.Stop()
	os.Exit(0)
}
