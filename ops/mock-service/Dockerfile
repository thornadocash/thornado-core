FROM python:3.12-alpine

WORKDIR /app
COPY mock_service.py /app/mock_service.py

EXPOSE 26656 26657 1317 9090 6040 7001 7101

CMD ["python", "/app/mock_service.py"]
