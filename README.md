# aws-ecs-eds ![Docker Pulls](https://img.shields.io/docker/pulls/boostchicken/aws-ecs-eds) ![GitHub Sponsors](https://img.shields.io/github/sponsors/boostchicken) ![GitHub Workflow Status (with event)](https://img.shields.io/github/actions/workflow/status/boostchicken/aws-ecs-eds/docker-publish.yml)

**Envoy EDS Service that automatically updates upstreams from AWS** 

# AWS Integrations

## AWS Elastic Container Service.

   Gets privateIPv4Address of EC2/Fargate Tasks.  +++++++
   ### Port Resolution
   1. Environmental Variable: **_aws.ecs.clusterName__port** (e.g. us-west-2-fargate_port=8080)
   2. Default: 80

## AWS Cloud Map

  Reads SRV records from CloudMap
    
  ### Port Resolution 
  1. Environmental Variable: **_aws.cloudMap.serviceDiscoveryId_**_port (e.g. srv-1234_port=8080)
  2. instance['AWS_INSTANCE_PORT'] from ListInstances CloudMap API
  3. Default:  80
         
# Envoy Config

   ### TCP Listener Config
   1. Environmental Variable: *EDS_LISTEN* (e.g. 127.0.0.1:8080)
   2. Default: 0.0.0.0:5678
   3. All responses cached for 30 seconds on successful response

### Config Snippet
[eds-config.yaml](https://github.com/boostchicken/aws-ecs-eds/blob/2b29f881b7f3cd592d3b601ef74b64053fff0d79/eds-config.yaml#L10-L57)https://github.com/boostchicken/aws-ecs-eds/blob/2b29f881b7f3cd592d3b601ef74b64053fff0d79/eds-config.yaml#L10-L57

### Custom Builds
[Dockerfile](https://github.com/boostchicken/aws-ecs-eds/blob/2b29f881b7f3cd592d3b601ef74b64053fff0d79/Dockerfile#L1-L13)https://github.com/boostchicken/aws-ecs-eds/blob/2b29f881b7f3cd592d3b601ef74b64053fff0d79/Dockerfile#L1-L13
