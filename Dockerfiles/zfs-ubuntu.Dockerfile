FROM ubuntu:latest

# Install ZFS packages
RUN apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    zfsutils-linux && \
    rm -rf /var/lib/apt/lists/*

CMD ["sleep", "infinity"]