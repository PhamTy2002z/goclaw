FROM python:3.12-slim
WORKDIR /app
RUN apt-get update && apt-get install -y --no-install-recommends curl && rm -rf /var/lib/apt/lists/*
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY main.py .
EXPOSE 11235
HEALTHCHECK --interval=30s --timeout=5s --retries=3 CMD curl -f http://localhost:11235/health || exit 1
CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "11235"]
