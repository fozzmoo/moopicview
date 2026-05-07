FROM alpine:latest AS app
RUN apk --no-cache add ca-certificates
WORKDIR /root/
# Copy pre-built binary from host
COPY moopicview-server ./moopicview
COPY DESIGN.md .
COPY frontend/dist ./frontend/dist
EXPOSE 8080
CMD ["./moopicview"]
