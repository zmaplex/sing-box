# 2404:c140
if ! curl -s https://android.chat.openai.com/cdn-cgi/trace | grep -q "2404:c140"; then
    echo "openai is locked"
    exit 1
fi
echo "openai is unlocked"

response=$(curl --socks5 127.0.0.1:1080 -o /dev/null -s -w "%{http_code}\n" https://www.netflix.com/sg-zh/title/70143836)

echo "$response https://www.netflix.com/sg-zh/title/70143836"
if [ "$response" -ne 200 ]; then
    echo "netflix is locked"
    exit 1
fi
echo "netflix is unlocked"