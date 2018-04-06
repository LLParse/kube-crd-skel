FROM ubuntu

RUN \
  apt-get update -y && \
  apt-get install -y arp-scan net-tools iputils-ping && \
  apt-get autoclean && \
  apt-get autoremove && \
  rm -rf /var/lib/apt/lists/*

ADD vm-controller /
ENTRYPOINT ["/vm-controller"]
CMD ["-v", "3"]
