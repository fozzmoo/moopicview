For local testing on this machine, we run the postgresql database in a
podman container and the moopicview application in its own podman
container. 

For "production" deployment, we run both the database and application
containers on the host 'tic' in docker containers and orchestrate the
containers with docker compose. The docker-compose.yml file is on tic in
the /home/fozz/work/ephemeraltic directory. 

Tic sits behind a Linux machine called lok which is connected to the
Internet. We have caddy running on lok which proxies traffic to tic on port
8787.

We can reach tic via ssh without a password. 

## Building and Deploying to tic

### Building the container image locally

We build the image on this machine using podman, then transfer it to tic.

The Go binary **must be statically linked** because the container uses
Alpine (musl libc). A dynamically-linked binary will fail with
`exec ./moopicview: no such file or directory` at runtime.

```
# Build a static Go binary
CGO_ENABLED=0 go build -o moopicview-server ./cmd/server/

# Build the container image (uses the Dockerfile in the repo root)
podman build --no-cache -t moopicview-local .
```

### Transferring the image to tic

```
# Export the image to a tar file
rm -f /tmp/moopicview-local.tar
podman save moopicview-local -o /tmp/moopicview-local.tar

# Copy to tic
scp /tmp/moopicview-local.tar tic:/tmp/moopicview-local.tar
```

### Deploying on tic

```
# Stop and remove the old container, remove the old image
ssh tic "docker kill moopicview; docker rm moopicview; docker rmi moopicview-local:latest localhost/moopicview-local:latest"

# Load the new image and tag it for docker compose
ssh tic "docker load -i /tmp/moopicview-local.tar && docker tag localhost/moopicview-local:latest moopicview-local:latest"

# Start the container
ssh tic "cd /home/fozz/work/ephemeraltic && docker compose up -d moopicview"

# Clean up
ssh tic "rm /tmp/moopicview-local.tar"
rm /tmp/moopicview-local.tar
```

The `docker tag` step is needed because `docker compose` looks for
`moopicview-local` (without the `localhost/` prefix that `docker load`
creates). Without it, compose will try to pull from a registry and fail.
