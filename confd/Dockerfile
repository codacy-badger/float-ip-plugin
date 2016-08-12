FROM python:2.7-alpine
# RUN apk add --no-cache curl 
#FROM python:2.7-wheezy
RUN pip install requests
ADD . /app/
CMD ["python", "/app/main.py"]
