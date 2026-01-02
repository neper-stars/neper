# Neper

This is an API server to automate the hosting of Stars! games.

It allows to create users. Each user can then upload stars races to its
user profile.
Each user can also start a new session or join an existing on
e if a slot is open.

The owner of the session will set some rules (generation scheduling, allowed
tactics like repair after gating...)
The owner will give the game options and this will generate the first turn
for all players.

# Quick start

Use the publicly available docker image by running the provided
docker-compose.yml file.

# Startup

## Nkeys

Neper is written to be able to be split into multiple components that can run
in parallel and work together. This requires a mechanism of communication.
We choose NATS.io as our internal communication backend.

And for the moment we decided to use simple preshared keys as a means
of authentication.

You will need to generate a Nats client key for the client code inside
the Neper server:

```shell
./neper generate-nkey
```

You will get 2 keys:
  - one private starting with an S which is a private key
    (needed by the client code of the neper server)
  - one public starting with an U which is a public key
    (needed if you have an *external* NATS server)

## Internal NATS server

If you want to just try out Neper without devops hassle,
you can just start neper with the internal NATS server (default mode)
using the following command line:

```shell
neper serve --scheme=http --nkey=SUAKNUB2...SYMKOA
```

Note: This is the private key for the client to be able to connect to
the Nats server. By default, a NATS server is started in the same process.
This server will derive the public key from the given private key,
and you have nothing else to do.

## External NATS server

When running multiple instances of Neper that will work together
you will want to have an external NATS server for microservices communications.

You will need to provide the public part of your new client key to
your server operator so that he may add this new key to the list
of authorized clients.

Then you can start your Neper server with the command line:

```shell
neper serve --scheme=http --nkey=SUAKNUB2...SYMKOA --no-nats-server --nats-url=https://mynatsserver:4222
```

We added two more options to the command line:
  - `--no-nats-server` to make sure neper does not try to start a NATS server
  - `--nats-url` to point the client code of Neper to the external NATS
    server you want to use
 
Note: This option is for heavy lifting when you want to host numerous
sessions with many clients, and is not needed if you just want to host
your own stars! sessions and play with a few friends.

## Player Requirements

All players MUST use the 2.6-jrc4 version of Stars! as this is the
version used to generate the turns.
