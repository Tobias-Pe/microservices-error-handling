import time
import requests

# simple repeated request

url = 'http://localhost/order'
for i in range(0, 1999):
    data = {
        "cartId": "1",
        "address": "Muster-Allee 42",
        "name": "Max Mustermann",
        "creditCard": "123456789-123",
        "email": "max.mustermann@web.de"
    }
    r = requests.post(url, json=data)
    print(i, r.status_code, r.text)
    r.close()
    time.sleep(0.001)
