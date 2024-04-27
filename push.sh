#  1469  git submodule update
#  1470  git submodule foreach git checkout main
#  1471  git submodule foreach git pull
CGO_ENABLED=0 go build -v -trimpath -ldflags "-X 'github.com/sagernet/sing-box/constant.Version=1.8.12-pro.0' -s -w -buildid=" -tags with_dhcp,with_quic,with_ech ./cmd/sing-box
aws s3 cp sing-box s3://$S3_SING_BOX_BUCKET --acl public-read --endpoint-url https://s3-accelerate.amazonaws.com