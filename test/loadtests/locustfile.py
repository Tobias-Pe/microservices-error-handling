from json import JSONDecodeError
from locust import HttpUser, task, between
import random


def decision(probability):
    return random.random() < probability


class OnlyLookingUser(HttpUser):
    exchangeLands = ['USD', 'GBP', 'INR', 'CAS', 'JPY', 'SEK', 'PLN']

    wait_time = between(0.5, 5)

    @task(1)
    def index(self):
        self.client.request_name = "/"
        with self.client.get("/", catch_response=True) as response:
            if not response.ok:
                response.failure(response.text)
        self.client.request_name = None

    @task(4)
    def articles(self):
        self.client.request_name = "/articles/"
        with self.client.get("/articles/", catch_response=True) as response:
            if not response.ok:
                response.failure(response.text)
        self.client.request_name = None

    @task(3)
    def exchange(self):
        self.client.request_name = "/exchange/${currency}"
        with self.client.get("/exchange/" + random.choice(self.exchangeLands), catch_response=True) as response:
            if not response.ok:
                response.failure(response.text)
        self.client.request_name = None


class CartUser(HttpUser):
    cart_id = -1
    to_be_bought = ""

    wait_time = between(0.5, 5)

    @task()
    def shop(self):
        self.get_article()
        if self.cart_id == -1:
            self.create_cart()
        else:
            self.add_to_cart()

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


class OrderUser(HttpUser):
    current_order_id = ""
    cart_id = -1
    to_be_bought = ""

    wait_time = between(0.5, 5)

    @task()
    def shop(self):
        if self.current_order_id == "":
            self.get_article()
            if self.cart_id == -1:
                self.create_cart()
            else:
                self.add_to_cart()
            if decision(0.1):
                self.order()
        else:
            self.check_order()

    def order(self):
        if self.cart_id != -1 and self.current_order_id == "":
            self.create_order()

    def create_order(self):
        self.client.request_name = "/order/"
        with self.client.post("/order/", json={
            "cartId": str(self.cart_id),
            "address": "Muster-Allee 42",
            "name": "Max Mustermann",
            "creditCard": "123456789-123",
            "email": "max.mustermann@web.de"
        }, catch_response=True) as response:
            if response.ok:
                try:
                    self.current_order_id = response.json()["order"]["orderId"]
                except JSONDecodeError:
                    response.failure("Response could not be decoded as JSON")
                except KeyError:
                    response.failure("Response did not contain expected key 'orderId'")
            else:
                response.failure(response.text)
        self.cart_id = -1
        self.client.request_name = None

    def check_order(self):
        self.client.request_name = "/order/${id}"
        with self.client.get("/order/" + self.current_order_id, catch_response=True) as response:
            if response.ok:
                try:
                    order_status = response.json()["order"]["status"]
                    if order_status == "COMPLETE" or order_status == "ABORTED":
                        self.current_order_id = ""
                except JSONDecodeError:
                    response.failure("Response could not be decoded as JSON")
                except KeyError:
                    response.failure("Response did not contain expected key 'status'")
            else:
                response.failure(response.text)
                self.current_order_id = ""
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
