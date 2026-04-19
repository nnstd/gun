import https from "node:https";

const KEY = `-----BEGIN PRIVATE KEY-----
MIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQC696+79z+4K+kW
oeEAqHf7OvhdiBX0k80r+SBxQVQNuPxIDXRgZwY8UxdD7uV+WKgpugdp1fCPvwKw
ZVYM7C7q4wXZo9zvJ84mZfeqi2DVllYNjgPl5ML/lMn3Of2vrB7lj2Y9EjN2awC6
KQrdlvCQmnLVaB5BBI0d9QzthVk5sOHhFRXT+/zzOibWL04BUKFHYeDp42s8YtkA
twEQMgO3BDX39uXcEvk7S7PY8Ng5jbcTetSHfLwNrHbV8HbbdxQvKYGqdVRcu7IF
vfU4ZlIhLKIv7/K8Ey4DamRAihE0oAxDAdoEOtiMh53/WmAEDC2NDKGl/0HSNv+6
zSwWrD5vAgMBAAECggEADTNi7pwmafp8gPyTX/Ieub5DV72kAmcgTvQbNkpGhP5R
BN7uUizz/VstT76UtYPM9VhzhyVz0Ev/awG8pNNU/LPKvFmCRkbcZ0zYj7ht8Q0M
RlvtXbrCvQjkUz0YJeJUZbqAnXWlH72uVdzplEhz88HAs3cGeJ2GcsPUH3ERO641
ccs8I+STmIPf67ldEcEXXJVZyi8rS+Hg6vOoua7+a8gYx+4KtUK0cq2PEiKCM8wy
k+qHeUZi2p+o4PMFNqYo/8Ewj1qJfjKItiZZR2enXXw9FATNTE+YwNzd8swPG18N
TXR4mDZgU3Qin9uTMvkGAs/8uAWBwNk8zAeRUrfXAQKBgQD9Jg8w7ZBLFOQLNXbG
FEwfUr27csBps7u2KjSERHTcqlcaKYx5yC3nGkbMjQjGZjRIACQxC9NR7sv23z9t
e1qmD713OZl3A6sBM+VPZrxKKXhn61xs3rQhlNyKySXS8MdifcWuVoZRFYGtBvMc
0Z2AVou8OwKXsSuidm3Trwx7IwKBgQC9EswdxzjS9Xgp/YHexMEVBw7nLEjEMpcz
1QRolVCIW0aG6gK1MIqY6avuv8fJETmRaj+GPO5R/L8FxbOHP0Kwt3eH7qbLVnPO
FP8Y16HHPZzOFQaGsDiHGGDycpbFd3eE9lqkEn950fO5Vfrr/x9lkKOfOJWeF8Lo
e7i4UpSaRQKBgQDZGSa2A0ZX3ZaktjkiLo4J3t+wPf0dqXI2C4P2Wu8Nv1frq+45
Ep+rLjHBgsIfw87aYKSpG0cjYPOyyEqRDdTzzVPjR5aBJrgk0+i4a5bW0zHbjVE6
XNOGaS+qJk811CBqKwq5NKMELrmDNg6QjIPSaGZ2CvVyOhL9xSry+5BsmQKBgQCt
M0H+aVh5j9njBVJpwm1pmMyjIiMKb5mpJpLiRx29u3dw8X9Hgc8E4tHYZKBcZUYK
Gn1UuA5M1q4aWI/r7hxmi7qYsBrlHC37c6p3lFijjqJM+l+/FEDEKKXukt/gxl6b
U52WvUc/Tf/pIIU6mLunK4dnvMr6RqQKmgON/kAYzQKBgErGLC/HEnd2UDfUhsoU
7e6CtKBEYCd7M2FOm1cxKwe9ngpEau5F5EVfZfu+nTfyJLsEzWRFvUL+7ye82HkU
uVrYuzu/prJfEOI0nj7JT7PaAsQOo+RxxNEGDjTfdaAzj2IKopnewS9xBBLbvEQW
wOg9d1JUKxo2w4mavqNDrpFP
-----END PRIVATE KEY-----
`;

const CERT = `-----BEGIN CERTIFICATE-----
MIIDJzCCAg+gAwIBAgIUWRIif7O/sYn6CdosVf3Ti0OfYdAwDQYJKoZIhvcNAQEL
BQAwFDESMBAGA1UEAwwJbG9jYWxob3N0MCAXDTI2MDQxOTA1NTQxN1oYDzIxMjYw
MzI2MDU1NDE3WjAUMRIwEAYDVQQDDAlsb2NhbGhvc3QwggEiMA0GCSqGSIb3DQEB
AQUAA4IBDwAwggEKAoIBAQC696+79z+4K+kWoeEAqHf7OvhdiBX0k80r+SBxQVQN
uPxIDXRgZwY8UxdD7uV+WKgpugdp1fCPvwKwZVYM7C7q4wXZo9zvJ84mZfeqi2DV
llYNjgPl5ML/lMn3Of2vrB7lj2Y9EjN2awC6KQrdlvCQmnLVaB5BBI0d9QzthVk5
sOHhFRXT+/zzOibWL04BUKFHYeDp42s8YtkAtwEQMgO3BDX39uXcEvk7S7PY8Ng5
jbcTetSHfLwNrHbV8HbbdxQvKYGqdVRcu7IFvfU4ZlIhLKIv7/K8Ey4DamRAihE0
oAxDAdoEOtiMh53/WmAEDC2NDKGl/0HSNv+6zSwWrD5vAgMBAAGjbzBtMB0GA1Ud
DgQWBBRHS96TRxyNi2AmDiJ5SpU/wvYr7TAfBgNVHSMEGDAWgBRHS96TRxyNi2Am
DiJ5SpU/wvYr7TAPBgNVHRMBAf8EBTADAQH/MBoGA1UdEQQTMBGCCWxvY2FsaG9z
dIcEfwAAATANBgkqhkiG9w0BAQsFAAOCAQEAml3Gso+NpXzrs0d7AJE9y8x1ISJI
uC0ZLkkPl4dHNVro20ZpT0jdtHSci24daDb3ET1aeFdtM8TA0LuwlPnG6yCte5fX
3Rf2ALwTP5nw+7lTXX8n92kz04yFLHs2pxQOrb2zcSLR0PT75BElUM/OLQ0k1/Yg
kpCu9yArNgHjs/NhgRakER2IJkM8+lgdP5HDfOD73Mo7jNEGhDSoL7fPE3rRUjPX
yEIs96ly6XY5RAcGbf1wPR41SUdWle6M1KX7M+FHnd+9OhKxeTRsRoULwodu5g7G
M2cnAeWKzU1QeIgvkrCrWFtYv37A6VmbC0m87IP3IkpdpLHN83CZECN8Tw==
-----END CERTIFICATE-----
`;

const srv = https.createServer({ key: KEY, cert: CERT }, (_req: any, res: any) => {
  res.end("tls-ok");
});

srv.listen(0, "127.0.0.1", () => {
  const a: any = srv.address();
  const port = a.port;
  const req = https.request(
    { host: "127.0.0.1", port: port, path: "/", rejectUnauthorized: false },
    (resp: any) => {
      let body = "";
      resp.on("data", (c: any) => { body += String(c); });
      resp.on("end", () => {
        console.log("status=" + resp.statusCode);
        console.log("body=" + body);
        srv.close();
      });
    },
  );
  req.end();
});
