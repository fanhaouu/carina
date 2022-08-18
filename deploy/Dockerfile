FROM harbor.shopeemobile.com/ecp/kubectl:1.20

COPY kubernetes /kubernetes

USER root

WORKDIR /kubernetes

ENTRYPOINT ["./deploy.sh"]
