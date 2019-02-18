docker build -f dev/Dockerfile -t klikuid/smsender .
docker rmi $(docker images -f "dangling=true" -q)