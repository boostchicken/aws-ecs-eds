FROM envoyproxy/envoy:v1.18.3
EXPOSE 8080
EXPOSE 9901
COPY eds-config.yaml /etc/envoy/envoy.yaml