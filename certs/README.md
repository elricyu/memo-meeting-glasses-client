# Trusted CA roots

The PEM bundle contains USERTrust ECC/RSA and ISRG Root X1/X2 public root certificates, sourced from the Debian ca-certificates trust store. It supports the default hosted instance without disabling TLS verification; the device clock must be correct. Root selection may need updating if the service changes its certificate chain.

For a self-hosted service, supply your trusted PEM bundle with `--ca`; never supply a private key. Licensing/provenance is recorded in `licenses/ca-certificates.txt`.
