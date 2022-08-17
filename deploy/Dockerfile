FROM bitnami/kubectl:1.20

COPY kubernetes /kubernetes

WORKDIR /kubernetes

ENTRYPOINT ["./deploy.sh"]
