FROM golang:1.16.4
ENV GOOS=linux
ENV GOARCH=amd64
COPY ./ /build
WORKDIR /build
RUN go mod vendor && go build -o aws-ecs-eds main.go

FROM amazonlinux:2
ENV EDS_LISTEN="0.0.0.0:5678"
EXPOSE 5678
WORKDIR /root/
COPY --from=0 /build/aws-ecs-eds /opt
CMD ["/opt/aws-ecs-eds"]