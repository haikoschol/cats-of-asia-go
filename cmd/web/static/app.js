// Copyright (C) 2023 Haiko Schol
// SPDX-License-Identifier: GPL-3.0-or-later

// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.

// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

const defaultZoomLevel = 15;
const maxZoomLevel = 22;
const defaultRadius = 12;

let favorites = null;
let images = [];
let map = null;

function makePopupContent(image, map) {
    const {id, urlSmall, urlLarge, timestamp} = image;
    const date = new Date(timestamp).toDateString();
    const location = formatLocation(image);
    const outer = document.createElement('div');
    const catImage = makeImageLink(urlLarge, urlSmall, `photo #${id}, showing one or more cats`);
    outer.appendChild(catImage);

    const footer = document.createElement('div');
    footer.className = 'popup-footer';

    const description = document.createElement('div');
    description.innerText = `Photo #${id}. Taken on ${date} in ${location}`;
    footer.appendChild(description);

    const favButton = makeFavoriteButton(id);
    favButton.mount(footer);

    if (navigator.share) {
        const shareButton = new IconButton(
            'static/share.svg',
            'share',
            () => shareCatto(id, map.getZoom())
        );

        shareButton.mount(footer);
    }

    outer.appendChild(footer);
    return outer;
}

function makeImageLink(href, src, alt) {
    const img = document.createElement('img');
    img.src = src;
    img.alt = alt;

    const a = document.createElement('a');
    a.href = href;
    a.appendChild(img);
    return a;
}

function makeFavoriteButton(imageId) {
    const icon = favorites.iconForStatus(imageId);
    const favButton = new IconButton(icon, 'add/remove this cat to/from your favorites');

    favButton.onclick = () => {
        favorites.toggle(imageId);
        favButton.src = favorites.iconForStatus(imageId);
        setFavoritesNavVisibility();
        favorites.write();
    }
    return favButton;
}

class IconButton {
    img;
    button;

    constructor(icon, alt, onClick) {
        this.img = document.createElement('img');
        this.img.className = 'icon';
        this.img.src = icon;
        this.img.alt = alt;

        this.button = document.createElement('button');

        if (onClick) {
            this.button.onclick = onClick;
        }

        this.button.appendChild(this.img);
    }

    set src(src) {
        this.img.src = src;
    }

    set onclick(oc) {
        this.button.onclick = oc;
    }

    mount(container) {
        container.appendChild(this.button);
    }
}

function setFavoritesNavVisibility() {
    const navItem = document.getElementById('favoritesNavItem');
    navItem.hidden = favorites.size === 0;
}

function shareCatto(imageId, zoomLevel) {
    const protocol = window.location.hostname === 'localhost' ? 'http' : 'https';
    const url = `${protocol}://${window.location.hostname}${window.location.pathname}?imageId=${imageId}&zoomLevel=${zoomLevel}`;

    navigator.share({
        title: `${document.title} #${imageId}`,
        text: 'Check out this cat!',
        url: url,
    })
        .then(() => console.log('catto sharing is catto caring'))
        .catch(error => console.log('error sharing:', error));
}

function formatLocation(image) {
    const {city, country} = image;
    return city ? `${city}, ${country}` : country
}

function addCircle(image, map, radius) {
    const color = image.randomized ? 'blue' : 'red';
    const circle = L.circle([image.latitude, image.longitude], {color: color, radius: radius});

    // Passing a function that returns dom elements in order to lazy load the popup images.
    const popup = circle.bindPopup(() => makePopupContent(image, map));

    circle.addTo(map);
    return circle;
}

function calculateRadius(zoomLevel) {
    let radius = defaultRadius;
    if (zoomLevel >= 17) {
        radius -= (zoomLevel % 16) * 2;
    }
    return Math.max(radius, 1);
}

function updateCircleRadii(images, zoomLevel) {
    const radius = calculateRadius(zoomLevel);
    images.forEach(img => img.circle.setRadius(radius));
}

// When multiple images have the same coordinates, spread them out so the markers won't overlap completely.
function adjustCoordinates(images) {
    const imgsByCoords = {};

    images.forEach(img => {
        img['randomized'] = false;
        const coords = `${img.latitude},${img.longitude}`;
        const imgsAt = imgsByCoords[coords] || [];
        imgsAt.push(img);
        imgsByCoords[coords] = imgsAt;
    });

    for (let c in imgsByCoords) {
        const imgsAt = imgsByCoords[c];
        const count = imgsAt.length;
        if (count < 2) {
            continue;
        }

        imgsAt.forEach(img => {
            img.latitude = randomizeCoordinate(img.latitude);
            img.longitude = randomizeCoordinate(img.longitude);
            img.randomized = true;
        });
    }
}

function randomizeCoordinate(coord) {
    const delta = Math.random() / 2000;
    return Math.random() > 0.5 ? coord + delta : coord - delta;
}

async function init(divId, accessToken) {
    favorites = new FavoriteStore();

    map = L.map(divId);
    L.tileLayer(`https://api.mapbox.com/styles/v1/{id}/tiles/{z}/{x}/{y}?access_token=${accessToken}`, {
        maxZoom: maxZoomLevel,
        id: 'mapbox/streets-v11',
        tileSize: 512,
        zoomOffset: -1
    }).addTo(map);

    const response = await fetch('/images/');
    images = await response.json();
    adjustCoordinates(images);

    const radius = calculateRadius(map.getZoom());
    images.forEach(img => img['circle'] = addCircle(img, map, radius));
    setMapView(map, images);

    map.on('zoomend', () => {
        updateCircleRadii(images, map.getZoom());
        updateCurrentPosition(map);
    });

    map.on('moveend', () => updateCurrentPosition(map));
    initPlaces(images, map);
    setFavoritesNavVisibility();
}

// If url params with image id and optional zoom level are present, center map on that, otherwise try last location
// from local storage and fall back to default values (coords of first image).
function setMapView(map, images) {
    let {latitude, longitude, zoomLevel} = getCurrentPosition(images[0].latitude, images[0].longitude)

    const urlParams = new URLSearchParams(window.location.search);
    const imageId = Number(urlParams.get('imageId'));
    const zoomParam = Number(urlParams.get('zoomLevel'));
    const imgsFromUrlParam = images.filter(img => img.id === imageId);

    if (imgsFromUrlParam.length === 1) {
        if (zoomParam <= maxZoomLevel && zoomParam >= 1) {
            zoomLevel = zoomParam;
        }
        const img = imgsFromUrlParam[0];

        // bit of a hack to remove the url params. probably would be better to update them if present
        history.pushState(null, '', window.location.pathname);

        map.setView([img.latitude, img.longitude], zoomLevel);
        img.circle.openPopup();
        updateCurrentPosition(map);
    } else {
        map.setView([latitude, longitude], zoomLevel);
    }

    updateCircleRadii(images, zoomLevel);
}

function initPlaces(images, map) {
    const placesUl = document.getElementById('placesUl');
    const places = {};

    images.forEach(img => places[formatLocation(img)] = img);
    const sorted = Object.keys(places).sort();

    for (const label of sorted) {
        const li = document.createElement('li');
        const a = document.createElement('a');
        const {latitude, longitude} = places[label];

        a.innerText = label;
        a.onclick = () => {
            document.getElementById('placesDropdown').removeAttribute('open');
            map.setView([latitude, longitude], defaultZoomLevel);
            updateCurrentPosition(map);
        }

        li.appendChild(a);
        placesUl.appendChild(li);
    }
}

function getCurrentPosition(startLatitude, startLongitude) {
    const lsLat = localStorage.getItem('latitude')
    const lsLng = localStorage.getItem('longitude')
    const lsZoom = localStorage.getItem('zoomLevel');

    return {
        latitude: lsLat ? Number(lsLat) : startLatitude,
        longitude: lsLng ? Number(lsLng) : startLongitude,
        zoomLevel: lsZoom ? Number(lsZoom) : defaultZoomLevel,
    };
}

function updateCurrentPosition(map) {
    const {lat, lng} = map.getCenter();

    localStorage.setItem('latitude', lat);
    localStorage.setItem('longitude', lng);
    localStorage.setItem('zoomLevel', map.getZoom());
}
