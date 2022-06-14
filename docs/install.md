# Running your own playground

This page describes how to deploy your own playground in a Docker environment.

Disclaimer: as the project is in the beta stage, we highly recommend 
running it in the playground-only infrastructure.

## Installing docker

Install Docker engine and docker-compose if you haven't already done so.

We've prepared a script to install docker tools on Ubuntu 20.04:
<details>
    <summary>init-ubuntu.sh</summary>

```bash
#!/bin/sh

sudo apt-get update -y
sudo apt-get install -y ca-certificates curl gnupg lsb-release

sudo mkdir -p /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg

echo \
  "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu \
  $(lsb_release -cs) stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null

sudo apt-get update -y
sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin docker-compose

# Check if everything is correct.
sudo docker run hello-world
docker-compose --version
```
</details>

Refer to the [official documentation](https://docs.docker.com/engine/install/#server) 
for guidelines for other distros.

## Running the playground back-end

At first, you should clone this repository and move to 
the `deploy` directory:
```bash
git clone git@github.com:lodthe/clickhouse-playground.git

# HTTPS version:
# git clone https://github.com/brandtbucher/specialist.git

cd clickhouse-playground/deploy
ls -lah
```

If you look around, you may find two important files:
- [config.yml](../deploy/config.yml) configures the playground.
- [docker-compose.yml](../deploy/docker-compose.yml) defines services and volumes required
  for launch the playground.

### DynamoDB

There is one thing in the default config that has to be changed:
AWS credentials. At the moment, the playground uses AWS DynamoDB to store
query execution results.

1. Create a new [DynamoDB](https://aws.amazon.com/dynamodb/) table with 
   the `Id` primary key (scalar string).
2. Create a [new IAM user](https://docs.aws.amazon.com/IAM/latest/UserGuide/id_users_create.html)
   and [allow access to the newly created DynamoDB table](https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_policies_examples_dynamodb_specific-table.html).
3. In the `config.yml` file find the `aws` field and fill credentials of the 
   created account, the table name and the chosen region.

### docker-compose.yml

The given docker-compose file defines the following services:
- **prometheus** for application and system metrics scraping;
- **cadvisor** for exporting resources usage by docker containers;
- **grafana** for metrics visualization;
- **playground** for serving user requests and running queries.

Services other than playground are supplementary and can be
commented/deleted. Also, there are commented services `nginx` and `certbot`.
You can uncomment these sections and configure proxying the way you like
(provide existing certificates or setup ACME).

You might have noticed that the host docker daemon socket is mounted in the 
playground container. The playground services needs access to the host
docker daemon to run ClickHouse containers locally. You may unmount 
the host socket and specify [remote runners](./remote-daemon.md).

### Running services

Now you are ready to run the playground: `docker-compose up -d`.
By default, services expose the following ports:
- **playground** &mdash; :9000.
- **grafana** &mdash; :3000 (login is `admin`, edit the `grafana/.env` file to change the password).
- **prometheus** &mdash; :9090.

You can check the status of services via `docker-compose ps` and 
see logs via `docker-compose logs -f <service name>`.

## Running the web application

TBD.
