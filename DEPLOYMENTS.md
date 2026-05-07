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
