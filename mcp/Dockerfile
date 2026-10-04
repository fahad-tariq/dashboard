FROM python:3.13-slim

# The container runs with a read-only root filesystem.
ENV PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1

WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY . .

USER 10001:10001
EXPOSE 9100

CMD ["python", "server.py"]
