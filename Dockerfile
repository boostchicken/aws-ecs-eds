FROM golang:alpine as builder
ENV GOOS=linux
ENV GOARCH=amd64
COPY ./ /build
WORKDIR /build
RUN go mod vendor && go get . && go build -o aws-ecs-eds main.go

FROM alpine
ENV EDS_LISTEN="0.0.0.0:5678"
EXPOSE 5678
WORKDIR /opt/
COPY --from=builder /build/aws-ecs-eds /opt/aws-ecs-eds
CMD ["/opt/aws-ecs-eds"]
