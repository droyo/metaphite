// Package certs loads CA certificates from a directory.
package certs

import (
	"crypto/x509"
	"encoding/pem"
	"io/ioutil"
)

type Pool []*x509.Certificate

// CertPool creates an x509.CertPool from a Pool,
// for use in a tls.Config struct.
func (p Pool) CertPool() *x509.CertPool {
	pool := x509.NewCertPool()
	for _, crt := range p {
		pool.AddCert(crt)
	}
	return pool
}

// Append adds zero or more certificates from extra pools
// to dst and returns dst.
func Append(dst Pool, extra ...Pool) Pool {
	for _, p := range extra {
		dst = append(dst, p...)
	}
	return dst
}

func fromFile(filename string) (Pool, error) {
	var pool Pool
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var block *pem.Block
	for {
		block, data = pem.Decode(data)
		if block == nil {
			break
		}
		crt, err := x509.ParseCertificates(block.Bytes)
		if err != nil {
			continue
		}
		pool = append(pool, crt...)
	}
	return pool, nil
}

// FromDir loads all PEM certificates in one or more directories.
func FromDir(directories ...string) Pool {
	var pool Pool
	for _, dir := range directories {
		fis, err := ioutil.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, f := range fis {
			pool = Append(pool, FromFile(f.Name()))
		}
	}
	return pool
}

// FromFile loads all PEM certificates from one or more files
func FromFile(files ...string) Pool {
	var pool Pool
	for _, name := range files {
		p, err := fromFile(name)
		if err != nil {
			continue
		}
		pool = Append(pool, p)
	}
	return pool
}
