CGO_ENABLED=0  go build -v -trimpath -ldflags "-X 'github.com/sagernet/sing-box/constant.Version=unknown' -s -w -buildid=" -tags with_dhcp,with_quic,with_ech ./cmd/sing-box 
aws s3 cp sing-box s3://$S3_SING_BOX_BUCKET --acl public-read
