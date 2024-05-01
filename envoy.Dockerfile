FROM envoyproxy/envoy:v1.28.0
EXPOSE 8080
EXPOSE 9901
COPY eds-config.yaml /etc/envoy/envoy.yaml