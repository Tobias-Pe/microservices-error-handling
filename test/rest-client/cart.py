import requests

url = 'http://localhost/cart/1'
for i in range(0, 7999):
    data = {"article_id": str(i)}
    r = requests.put(url, json=data)
    print(i, r.status_code)
    r.close()
