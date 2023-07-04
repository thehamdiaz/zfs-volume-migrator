FROM ubuntu:latest

# Install ZFS packages
RUN apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    zfsutils-linux && \
    rm -rf /var/lib/apt/lists/*

# Install SSH client
RUN apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    openssh-client && \
    rm -rf /var/lib/apt/lists/*

RUN mkdir -p /root/.ssh 
RUN chmod 600 /root/.ssh 
COPY entrypoint.sh /usr/local/bin/
RUN chmod +x /usr/local/bin/entrypoint.sh

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]