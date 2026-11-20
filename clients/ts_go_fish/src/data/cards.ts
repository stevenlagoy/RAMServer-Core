import ace_hearts     from "@assets/cards/ace_hearts.png";
import two_hearts     from "@assets/cards/two_hearts.png";
import three_hearts   from "@assets/cards/three_hearts.png";
import four_hearts    from "@assets/cards/four_hearts.png";
import five_hearts    from "@assets/cards/five_hearts.png";
import six_hearts     from "@assets/cards/six_hearts.png";
import seven_hearts   from "@assets/cards/seven_hearts.png";
import eight_hearts   from "@assets/cards/eight_hearts.png";
import nine_hearts    from "@assets/cards/nine_hearts.png";
import ten_hearts     from "@assets/cards/ten_hearts.png";
import jack_hearts    from "@assets/cards/jack_hearts.png";
import queen_hearts   from "@assets/cards/queen_hearts.png";
import king_hearts    from "@assets/cards/king_hearts.png";
import ace_spades     from "@assets/cards/ace_spades.png";
import two_spades     from "@assets/cards/two_spades.png";
import three_spades   from "@assets/cards/three_spades.png";
import four_spades    from "@assets/cards/four_spades.png";
import five_spades    from "@assets/cards/five_spades.png";
import six_spades     from "@assets/cards/six_spades.png";
import seven_spades   from "@assets/cards/seven_spades.png";
import eight_spades   from "@assets/cards/eight_spades.png";
import nine_spades    from "@assets/cards/nine_spades.png";
import ten_spades     from "@assets/cards/ten_spades.png";
import jack_spades    from "@assets/cards/jack_spades.png";
import queen_spades   from "@assets/cards/queen_spades.png";
import king_spades    from "@assets/cards/king_spades.png";
import ace_diamonds   from "@assets/cards/ace_diamonds.png";
import two_diamonds   from "@assets/cards/two_diamonds.png";
import three_diamonds from "@assets/cards/three_diamonds.png";
import four_diamonds  from "@assets/cards/four_diamonds.png";
import five_diamonds  from "@assets/cards/five_diamonds.png";
import six_diamonds   from "@assets/cards/six_diamonds.png";
import seven_diamonds from "@assets/cards/seven_diamonds.png";
import eight_diamonds from "@assets/cards/eight_diamonds.png";
import nine_diamonds  from "@assets/cards/nine_diamonds.png";
import ten_diamonds   from "@assets/cards/ten_diamonds.png";
import jack_diamonds  from "@assets/cards/jack_diamonds.png";
import queen_diamonds from "@assets/cards/queen_diamonds.png";
import king_diamonds  from "@assets/cards/king_diamonds.png";
import ace_clubs      from "@assets/cards/ace_clubs.png";
import two_clubs      from "@assets/cards/two_clubs.png";
import three_clubs    from "@assets/cards/three_clubs.png";
import four_clubs     from "@assets/cards/four_clubs.png";
import five_clubs     from "@assets/cards/five_clubs.png";
import six_clubs      from "@assets/cards/six_clubs.png";
import seven_clubs    from "@assets/cards/seven_clubs.png";
import eight_clubs    from "@assets/cards/eight_clubs.png";
import nine_clubs     from "@assets/cards/nine_clubs.png";
import ten_clubs      from "@assets/cards/ten_clubs.png";
import jack_clubs     from "@assets/cards/jack_clubs.png";
import queen_clubs    from "@assets/cards/queen_clubs.png";
import king_clubs     from "@assets/cards/king_clubs.png";

type Card = {
    imgSrc: string,
    name: string,
    rank: string,
    value: Number,
    suit: string,
    symbol: string,
};

export const cards = new Map<string, Card>([
    ["Ace of Hearts", {
        imgSrc: ace_hearts,
        name: "Ace of Hearts",
        rank: "Ace",
        value: 1,
        suit: "Hearts",
        symbol: "A♣",
    }],
    ["Two of Hearts", {
        imgSrc: two_hearts,
        name: "Two of Hearts",
        rank: "Two",
        value: 2,
        suit: "Hearts",
        symbol: "2♣",
    }],
    ["Three of Hearts", {
        imgSrc: three_hearts,
        name: "Three of Hearts",
        rank: "Three",
        value: 3,
        suit: "Hearts",
        symbol: "3♣",
    }],
    ["Four of Hearts", {
        imgSrc: four_hearts,
        name: "Four of Hearts",
        rank: "Four",
        value: 4,
        suit: "Hearts",
        symbol: "4♣",
    }],
    ["Five of Hearts", {
        imgSrc: five_hearts,
        name: "Five of Hearts",
        rank: "Five",
        value: 5,
        suit: "Hearts",
        symbol: "5♣",
    }],
    ["Six of Hearts", {
        imgSrc: six_hearts,
        name: "Six of Hearts",
        rank: "Six",
        value: 6,
        suit: "Hearts",
        symbol: "6♣",
    }],
    ["Seven of Hearts", {
        imgSrc: seven_hearts,
        name: "Seven of Hearts",
        rank: "Seven",
        value: 7,
        suit: "Hearts",
        symbol: "7♣",
    }],
    ["Eight of Hearts", {
        imgSrc: eight_hearts,
        name: "Eight of Hearts",
        rank: "Eight",
        value: 8,
        suit: "Hearts",
        symbol: "8♣",
    }],
    ["Nine of Hearts", {
        imgSrc: nine_hearts,
        name: "Nine of Hearts",
        rank: "Nine",
        value: 9,
        suit: "Hearts",
        symbol: "9♣",
    }],
    ["Ten of Hearts", {
        imgSrc: ten_hearts,
        name: "Ten of Hearts",
        rank: "Ten",
        value: 10,
        suit: "Hearts",
        symbol: "10♣",
    }],
    ["Jack of Hearts", {
        imgSrc: jack_hearts,
        name: "Jack of Hearts",
        rank: "Jack",
        value: 11,
        suit: "Hearts",
        symbol: "J♣",
    }],
    ["Queen of Hearts", {
        imgSrc: queen_hearts,
        name: "Queen of Hearts",
        rank: "Queen",
        value: 12,
        suit: "Hearts",
        symbol: "Q♣",
    }],
    ["King of Hearts", {
        imgSrc: king_hearts,
        name: "King of Hearts",
        rank: "King",
        value: 13,
        suit: "Hearts",
        symbol: "K♣",
    }],
    ["Ace of Spades", {
        imgSrc: ace_spades,
        name: "Ace of Spades",
        rank: "Ace",
        value: 1,
        suit: "Spades",
        symbol: "A♣",
    }],
    ["Two of Spades", {
        imgSrc: two_spades,
        name: "Two of Spades",
        rank: "Two",
        value: 2,
        suit: "Spades",
        symbol: "2♣",
    }],
    ["Three of Spades", {
        imgSrc: three_spades,
        name: "Three of Spades",
        rank: "Three",
        value: 3,
        suit: "Spades",
        symbol: "3♣",
    }],
    ["Four of Spades", {
        imgSrc: four_spades,
        name: "Four of Spades",
        rank: "Four",
        value: 4,
        suit: "Spades",
        symbol: "4♣",
    }],
    ["Five of Spades", {
        imgSrc: five_spades,
        name: "Five of Spades",
        rank: "Five",
        value: 5,
        suit: "Spades",
        symbol: "5♣",
    }],
    ["Six of Spades", {
        imgSrc: six_spades,
        name: "Six of Spades",
        rank: "Six",
        value: 6,
        suit: "Spades",
        symbol: "6♣",
    }],
    ["Seven of Spades", {
        imgSrc: seven_spades,
        name: "Seven of Spades",
        rank: "Seven",
        value: 7,
        suit: "Spades",
        symbol: "7♣",
    }],
    ["Eight of Spades", {
        imgSrc: eight_spades,
        name: "Eight of Spades",
        rank: "Eight",
        value: 8,
        suit: "Spades",
        symbol: "8♣",
    }],
    ["Nine of Spades", {
        imgSrc: nine_spades,
        name: "Nine of Spades",
        rank: "Nine",
        value: 9,
        suit: "Spades",
        symbol: "9♣",
    }],
    ["Ten of Spades", {
        imgSrc: ten_spades,
        name: "Ten of Spades",
        rank: "Ten",
        value: 10,
        suit: "Spades",
        symbol: "10♣",
    }],
    ["Jack of Spades", {
        imgSrc: jack_spades,
        name: "Jack of Spades",
        rank: "Jack",
        value: 11,
        suit: "Spades",
        symbol: "J♣",
    }],
    ["Queen of Spades", {
        imgSrc: queen_spades,
        name: "Queen of Spades",
        rank: "Queen",
        value: 12,
        suit: "Spades",
        symbol: "Q♣",
    }],
    ["King of Spades", {
        imgSrc: king_spades,
        name: "King of Spades",
        rank: "King",
        value: 13,
        suit: "Spades",
        symbol: "K♣",
    }],
    ["Ace of Diamonds", {
        imgSrc: ace_diamonds,
        name: "Ace of Diamonds",
        rank: "Ace",
        value: 1,
        suit: "Diamonds",
        symbol: "A♣",
    }],
    ["Two of Diamonds", {
        imgSrc: two_diamonds,
        name: "Two of Diamonds",
        rank: "Two",
        value: 2,
        suit: "Diamonds",
        symbol: "2♣",
    }],
    ["Three of Diamonds", {
        imgSrc: three_diamonds,
        name: "Three of Diamonds",
        rank: "Three",
        value: 3,
        suit: "Diamonds",
        symbol: "3♣",
    }],
    ["Four of Diamonds", {
        imgSrc: four_diamonds,
        name: "Four of Diamonds",
        rank: "Four",
        value: 4,
        suit: "Diamonds",
        symbol: "4♣",
    }],
    ["Five of Diamonds", {
        imgSrc: five_diamonds,
        name: "Five of Diamonds",
        rank: "Five",
        value: 5,
        suit: "Diamonds",
        symbol: "5♣",
    }],
    ["Six of Diamonds", {
        imgSrc: six_diamonds,
        name: "Six of Diamonds",
        rank: "Six",
        value: 6,
        suit: "Diamonds",
        symbol: "6♣",
    }],
    ["Seven of Diamonds", {
        imgSrc: seven_diamonds,
        name: "Seven of Diamonds",
        rank: "Seven",
        value: 7,
        suit: "Diamonds",
        symbol: "7♣",
    }],
    ["Eight of Diamonds", {
        imgSrc: eight_diamonds,
        name: "Eight of Diamonds",
        rank: "Eight",
        value: 8,
        suit: "Diamonds",
        symbol: "8♣",
    }],
    ["Nine of Diamonds", {
        imgSrc: nine_diamonds,
        name: "Nine of Diamonds",
        rank: "Nine",
        value: 9,
        suit: "Diamonds",
        symbol: "9♣",
    }],
    ["Ten of Diamonds", {
        imgSrc: ten_diamonds,
        name: "Ten of Diamonds",
        rank: "Ten",
        value: 10,
        suit: "Diamonds",
        symbol: "10♣",
    }],
    ["Jack of Diamonds", {
        imgSrc: jack_diamonds,
        name: "Jack of Diamonds",
        rank: "Jack",
        value: 11,
        suit: "Diamonds",
        symbol: "J♣",
    }],
    ["Queen of Diamonds", {
        imgSrc: queen_diamonds,
        name: "Queen of Diamonds",
        rank: "Queen",
        value: 12,
        suit: "Diamonds",
        symbol: "Q♣",
    }],
    ["King of Diamonds", {
        imgSrc: king_diamonds,
        name: "King of Diamonds",
        rank: "King",
        value: 13,
        suit: "Diamonds",
        symbol: "K♣",
    }],
    ["Ace of Clubs", {
        imgSrc: ace_clubs,
        name: "Ace of Clubs",
        rank: "Ace",
        value: 1,
        suit: "Clubs",
        symbol: "A♣",
    }],
    ["Two of Clubs", {
        imgSrc: two_clubs,
        name: "Two of Clubs",
        rank: "Two",
        value: 2,
        suit: "Clubs",
        symbol: "2♣",
    }],
    ["Three of Clubs", {
        imgSrc: three_clubs,
        name: "Three of Clubs",
        rank: "Three",
        value: 3,
        suit: "Clubs",
        symbol: "3♣",
    }],
    ["Four of Clubs", {
        imgSrc: four_clubs,
        name: "Four of Clubs",
        rank: "Four",
        value: 4,
        suit: "Clubs",
        symbol: "4♣",
    }],
    ["Five of Clubs", {
        imgSrc: five_clubs,
        name: "Five of Clubs",
        rank: "Five",
        value: 5,
        suit: "Clubs",
        symbol: "5♣",
    }],
    ["Six of Clubs", {
        imgSrc: six_clubs,
        name: "Six of Clubs",
        rank: "Six",
        value: 6,
        suit: "Clubs",
        symbol: "6♣",
    }],
    ["Seven of Clubs", {
        imgSrc: seven_clubs,
        name: "Seven of Clubs",
        rank: "Seven",
        value: 7,
        suit: "Clubs",
        symbol: "7♣",
    }],
    ["Eight of Clubs", {
        imgSrc: eight_clubs,
        name: "Eight of Clubs",
        rank: "Eight",
        value: 8,
        suit: "Clubs",
        symbol: "8♣",
    }],
    ["Nine of Clubs", {
        imgSrc: nine_clubs,
        name: "Nine of Clubs",
        rank: "Nine",
        value: 9,
        suit: "Clubs",
        symbol: "9♣",
    }],
    ["Ten of Clubs", {
        imgSrc: ten_clubs,
        name: "Ten of Clubs",
        rank: "Ten",
        value: 10,
        suit: "Clubs",
        symbol: "10♣",
    }],
    ["Jack of Clubs", {
        imgSrc: jack_clubs,
        name: "Jack of Clubs",
        rank: "Jack",
        value: 11,
        suit: "Clubs",
        symbol: "J♣",
    }],
    ["Queen of Clubs", {
        imgSrc: queen_clubs,
        name: "Queen of Clubs",
        rank: "Queen",
        value: 12,
        suit: "Clubs",
        symbol: "Q♣",
    }],
    ["King of Clubs", {
        imgSrc: king_clubs,
        name: "King of Clubs",
        rank: "King",
        value: 13,
        suit: "Clubs",
        symbol: "K♣",
    }],
])