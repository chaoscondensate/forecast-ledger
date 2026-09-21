# FreeTSA RFC 3161 interoperability fixture

These byte-exact files contain no private forecast data. They were retrieved on
2026-09-21 from the qualified `https://freetsa.org/tsr` profile with OpenSSL
3.6.0:

```sh
openssl ts -query -sha256 -cert -data target.txt -out request.tsq
curl --fail --silent --show-error --max-redirs 0 \
  --header 'Content-Type: application/timestamp-query' \
  --header 'Accept: application/timestamp-reply' \
  --data-binary '@request.tsq' \
  --output response.tsr \
  https://freetsa.org/tsr
openssl ts -verify \
  -queryfile request.tsq \
  -in response.tsr \
  -CAfile ../../providers/freetsa/ca.pem \
  -untrusted tsa.pem
```

OpenSSL reports `Verification: OK`. The response uses a SHA-256 message
imprint, matching nonce, ESS `SigningCertificate` v1, SHA-512 CMS digest, and
ECDSA-with-SHA-512 signature.

| File | SHA-256 |
| --- | --- |
| `target.txt` | `e232db4b3ef9609976f34a6abc70a56faf39e6811f6f5d2a17655afc039aa391` |
| `request.tsq` | `4c9d7150bcbbf1be186c393517a7f0f84e0d569e34101839f8059ea185d5a25b` |
| `response.tsr` | `1f86cb9efdbf1ab088a0f50484448efd80dc2210392f7136ae15f420a2a37004` |
| `tsa.pem` | `8bfb0305bb64e2571ca507552ef3245cb1c2fee8728e0ff8689225081ea13467` |

The exact root bundle is
`internal/timestamp/rfc3161/providers/freetsa/ca.pem`, SHA-256
`2151b61137ffa86bf664691ba67e7da0b19f98c758e3d228d5d8ebf27e044438`.
Normal tests use these retained bytes and never contact the live service.
