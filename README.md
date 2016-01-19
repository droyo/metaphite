![build status](https://travis-ci.org/droyo/metaphite.svg?branch=master)
# metaphite: stitch multiple graphite servers together

It is not uncommon to run separate graphite installations for
separate environments. For example, separate "dev" and "production"
graphite clusters. However, the reason graphite became so succesful
because it provides a friendly API to query *all* of your metrics.
With metaphite, you can hide your disparate servers behind a
single endpoint.

# Installation

If you have a Go compiler (version 1.3 or above) installed, run

	go get github.com/droyo/metaphite

Binary releases will be provided at a later time.

# Setup

Create a file `config.json`, containing mappings, like so:

	{
		"mappings": {
			"qe": "http://qe-graphite.example.net/",
			"dev": "http://dev-graphite.example.net/"
		}
	}

To run `metaphite`, execute

	metaphite -c config.json -http=:8080

metaphite will log http requests to standard error in
the Common Log Format.

# Usage

With metaphite listening on http://localhost:8080 , open a
`render` query in your browser:

	`open http://localhost:12036/render?target=aliasByMetric%28scale%dev.servers.messagebus01.rabbitmq.object_totals.{queues,exchanges,consumers,channels,connections},%202%29%29`

You should see a graph rendered by the server specified for your
`dev` mapping in your configuration.
