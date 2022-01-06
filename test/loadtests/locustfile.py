import random
import time
from json import JSONDecodeError

from locust import HttpUser, between, task


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
            "cartId": cart_id,
            "address": "-",  # no address
            "name": "Max Mustermann",
            "creditCard": "123456789-123",
            "email": "max.mustermann@web.de"
        },
        {
            "cartId": cart_id,
            "address": "Muster-Allee 42",
            "name": "Max Mustermann",
            "creditCard": "nocreditcard",
            "email": "max.mustermann@web.de"
        },
        {
            "cartId": cart_id,
            "address": "Muster-Allee 42",
            "name": "Max Mustermann",
            "creditCard": "123456789-123",
            "email": "noemailaddress"
        },
        {
            "cartId": "cartWithUnknownArticlesToBeFetched",
            "address": "Muster-Allee 42",
            "name": "Max Mustermann",
            "creditCard": "123456789-123",
            "email": "max.mustermann@web.de"
        },
    ]
    return random.choice(failure_datas)


class OrderHttpUser:

    def create_order(self, order_data) -> str:
        order_id = ""
        self.client.request_name = "/order/"
        with self.client.post("/order/", json=order_data, catch_response=True) as response:
            if response.ok:
                try:
                    order_id = response.json()["order"]["orderId"]
                except JSONDecodeError:
                    response.failure("Response could not be decoded as JSON")
                except KeyError:
                    response.failure("Response did not contain expected key 'orderId'")
                except Exception as e:
                    response.failure(e)
            else:
                if order_data["email"] == "noemailaddress":
                    response.success()
                else:
                    response.failure(response.text)
        self.client.request_name = None
        return order_id

    def check_order(self, order_id: str) -> str:
        order_status = ""
        self.client.request_name = "/order/${id}"
        with self.client.get("/order/" + order_id, catch_response=True) as response:
            if response.ok:
                try:
                    order_status = response.json()["order"]["status"]
                except JSONDecodeError:
                    response.failure("Response could not be decoded as JSON")
                except KeyError:
                    response.failure("Response did not contain expected key 'status'")
                except Exception as e:
                    response.failure(e)
            else:
                response.failure(response.text)
        self.client.request_name = None
        return order_status

    def get_exchange(self):
        self.client.request_name = "/exchange/${currency}"
        try:
            with self.client.get("/exchange/" + get_exchange_land(), catch_response=True) as response:
                if not response.ok:
                    response.failure(response.text)
        except Exception as e:
            response.failure(e)
        self.client.request_name = None

    def get_article(self) -> str:
        article_id = ""
        self.client.request_name = "/articles/"
        with self.client.get("/articles/", catch_response=True) as response:
            if response.ok:
                try:
                    article_id = random.choice(response.json()["articles"])["id"]
                    if len(article_id) == 0:
                        response.failure("empty article id")
                except JSONDecodeError:
                    response.failure("Response could not be decoded as JSON")
                except KeyError:
                    response.failure("Response did not contain expected key 'id'")
                except Exception as e:
                    response.failure(e)
            else:
                response.failure(response.text)
        self.client.request_name = None
        return article_id

    def create_cart(self, article_id: str) -> str:
        cart_id = ""
        self.client.request_name = "/cart"
        with self.client.post("/cart", json={"article_id": article_id}, catch_response=True) as response:
            if response.ok:
                try:
                    cart_id = response.json()["cart"]["cartId"]
                except JSONDecodeError:
                    response.failure("Response could not be decoded as JSON")
                except KeyError:
                    response.failure("Response did not contain expected key 'cartId'")
                except Exception as e:
                    response.failure(e)
            else:
                response.failure(response.text)
        self.client.request_name = None
        return cart_id

    def add_to_cart(self, cart_id: str, article_id: str):
        self.client.request_name = "/cart/${id}"
        with self.client.put("/cart/" + cart_id, json={"article_id": article_id},
                             catch_response=True) as response:
            if not response.ok:
                response.failure(response.text)
        self.client.request_name = None

    def get_cart(self, cart_id: str):
        self.client.request_name = "/cart/${id}"
        with self.client.get("/cart/" + cart_id, catch_response=True) as response:
            if not response.ok:
                response.failure(response.text)
        self.client.request_name = None


class NoWaitOrderUser(OrderHttpUser, HttpUser):
    weight = 5
    wait_time = between(0.5, 5)

    @task()
    def shopping(self):
        self.exchange()

    def exchange(self):
        if decision(0.4):
            self.get_exchange()
        self.populate_cart()

    def populate_cart(self):
        cart_id = ""
        while cart_id == "" or decision(0.9):
            article = self.get_article()
            if len(article) > 0:
                if cart_id == "":
                    cart_id = self.create_cart(article)
                else:
                    self.add_to_cart(cart_id=cart_id, article_id=article)
            if cart_id != "" and decision(0.5):
                self.get_cart(cart_id)
            time.sleep(random.uniform(0.5, 5))

        self.create_nowait_order(cart_id)

    def create_nowait_order(self, cart_id):
        customer_data = get_customer_data(cart_id)
        if customer_data["cartId"] == "cartWithUnknownArticlesToBeFetched":
            customer_data["cartId"] = self.create_cart("iDoNotExist")
        self.create_order(customer_data)


class WaitingOrderUser(OrderHttpUser, HttpUser):
    weight = 5
    wait_time = between(0.5, 5)

    @task()
    def shopping(self):
        self.exchange()

    def exchange(self):
        if decision(0.4):
            self.get_exchange()
        self.populate_cart()

    def populate_cart(self):
        cart_id = ""
        while cart_id == "" or decision(0.9):
            article = self.get_article()
            if len(article) > 0:
                if cart_id == "":
                    cart_id = self.create_cart(article)
                else:
                    self.add_to_cart(cart_id=cart_id, article_id=article)
            if cart_id != "" and decision(0.5):
                self.get_cart(cart_id)
            time.sleep(random.uniform(0.5, 5))

        self.create_wait_order(cart_id)

    def create_wait_order(self, cart_id):
        order_id = ""
        order_status = ""
        while not (order_status == "COMPLETE" or order_status == "ABORTED"):
            if order_id == "":
                customer_data = get_customer_data(cart_id)
                if customer_data["cartId"] == "cartWithUnknownArticlesToBeFetched":
                    customer_data["cartId"] = self.create_cart("iDoNotExist")
                order_id = self.create_order(customer_data)
            else:
                order_status = self.check_order(order_id)
                print(order_status)
            time.sleep(random.uniform(0.5, 5))
