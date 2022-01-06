import random
from json import JSONDecodeError

from locust import HttpUser, task, between


def decision(probability):
    return random.random() < probability


def get_exchange_land():
    exchange_lands = ['USD', 'GBP', 'INR', 'CAS', 'JPY', 'SEK', 'PLN']
    return random.choice(exchange_lands)


def get_customer_data(cart_id):
    if decision(0.1):
        return failure_data(cart_id)

    return success_data(cart_id)


def success_data(cart_id):
    return {
        "cartId": str(cart_id),
        "address": "Muster-Allee 42",
        "name": "Max Mustermann",
        "creditCard": "123456789-123",
        "email": "max.mustermann@web.de"
    }


def failure_data(cart_id):
    failure_datas = [
        {
            "cartId": "thiscartdoesnotexist",
            "address": "Muster-Allee 42",
            "name": "Max Mustermann",
            "creditCard": "123456789-123",
            "email": "max.mustermann@web.de"
        },
        {
            "cartId": str(cart_id),
            "address": "-",  # no address
            "name": "Max Mustermann",
            "creditCard": "123456789-123",
            "email": "max.mustermann@web.de"
        },
        {
            "cartId": str(cart_id),
            "address": "Muster-Allee 42",
            "name": "Max Mustermann",
            "creditCard": "nocreditcard",
            "email": "max.mustermann@web.de"
        },
        {
            "cartId": str(cart_id),
            "address": "Muster-Allee 42",
            "name": "Max Mustermann",
            "creditCard": "123456789-123",
            "email": "noemailaddress"
        },
    ]
    return random.choice(failure_datas)


class OrderUser(HttpUser):
    weight = 6
    current_waiting_order_id = ""
    cart_id = -1
    to_be_bought = ""

    wait_time = between(0.5, 5)

    @task(1)
    def exchange(self):
        self.get_exchange()

    @task(9)
    def shop_and_wait_with_success_order_data(self):
        if self.current_waiting_order_id == "":
            self.get_article()
            if self.cart_id == -1:
                self.create_cart()
            else:
                self.add_to_cart()
            if self.cart_id != -1 and self.current_waiting_order_id == "" and decision(0.1):
                self.create_success_order()
        else:
            self.check_order()

    @task(6)
    def shop_with_random_order_data(self):
        self.get_article()
        if self.cart_id == -1:
            self.create_cart()
        else:
            self.add_to_cart()
        if self.cart_id != -1 and decision(0.4):
            self.create_random_order()

    def create_random_order(self):
        self.client.request_name = "/order/"
        customer_data = get_customer_data(cart_id=self.cart_id)
        with self.client.post("/order/", json=customer_data, catch_response=True) as response:
            if not response.ok:
                if customer_data["email"] != "noemailaddress":
                    response.failure(response.text + customer_data["email"])
                else:
                    response.success()
        self.cart_id = -1
        self.client.request_name = None

    def create_success_order(self):
        self.client.request_name = "/order/"
        customer_data = get_customer_data(cart_id=self.cart_id)
        with self.client.post("/order/", json=customer_data, catch_response=True) as response:
            if response.ok:
                try:
                    self.current_waiting_order_id = response.json()["order"]["orderId"]
                except JSONDecodeError:
                    response.failure("Response could not be decoded as JSON")
                except KeyError:
                    response.failure("Response did not contain expected key 'orderId'")
                except Exception as e:
                    response.failure(e)
            else:
                if customer_data["email"] != "noemailaddress":
                    response.failure(response.text + customer_data["email"])
                else:
                    response.success()
        self.cart_id = -1
        self.client.request_name = None

    def check_order(self):
        self.client.request_name = "/order/${id}"
        with self.client.get("/order/" + self.current_waiting_order_id, catch_response=True) as response:
            if response.ok:
                try:
                    order_status = response.json()["order"]["status"]
                    if order_status == "COMPLETE" or order_status == "ABORTED":
                        self.current_waiting_order_id = ""
                except JSONDecodeError:
                    response.failure("Response could not be decoded as JSON")
                except KeyError:
                    response.failure("Response did not contain expected key 'status'")
                except Exception as e:
                    response.failure(e)
            else:
                response.failure(response.text)
                self.current_waiting_order_id = ""
        self.client.request_name = None

    def get_exchange(self):
        self.client.request_name = "/exchange/${currency}"
        try:
            with self.client.get("/exchange/" + get_exchange_land(), catch_response=True) as response:
                if not response.ok:
                    response.failure(response.text)
        except Exception as e:
            response.failure(e)
        self.client.request_name = None

    def get_article(self):
        self.client.request_name = "/articles/"
        with self.client.get("/articles/", catch_response=True) as response:
            if response.ok:
                try:
                    self.to_be_bought = random.choice(response.json()["articles"])["id"]
                except JSONDecodeError:
                    response.failure("Response could not be decoded as JSON")
                except KeyError:
                    response.failure("Response did not contain expected key 'id'")
                except Exception as e:
                    response.failure(e)
            else:
                response.failure(response.text)
        self.client.request_name = None

    def create_cart(self):
        self.client.request_name = "/cart"
        with self.client.post("/cart", json={"article_id": self.to_be_bought}, catch_response=True) as response:
            if response.ok:
                try:
                    self.cart_id = int(response.json()["cart"]["cartId"])
                except JSONDecodeError:
                    response.failure("Response could not be decoded as JSON")
                except KeyError:
                    response.failure("Response did not contain expected key 'cartId'")
                except Exception as e:
                    response.failure(e)
            else:
                response.failure(response.text)
        self.client.request_name = None

    def add_to_cart(self):
        self.client.request_name = "/cart/${id}"
        with self.client.put("/cart/" + str(self.cart_id), json={"article_id": self.to_be_bought},
                             catch_response=True) as response:
            if not response.ok:
                response.failure(response.text)
        self.client.request_name = None
