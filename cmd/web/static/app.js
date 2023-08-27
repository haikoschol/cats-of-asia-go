const defaultZoomLevel = 15;
const maxZoomLevel = 22;
const defaultRadius = 12;
let images = [];
let map = null;

function makePopupContent(image, map) {
    const {id, timestamp} = image;
    const date = new Date(timestamp).toDateString();
    const location = formatLocation(image);
    const outer = document.createElement('div');
    const catImage = makeImageLink(`images/${id}`, `images/${id}`, 'a photo of one or more cats');
    const description = document.createElement('div');

    description.innerText = `Taken on ${date} in ${location}`;
    outer.appendChild(catImage);

    if (navigator.share) {
        const footer = document.createElement('div');
        footer.className = 'popup-footer';
        footer.appendChild(description);
        footer.appendChild(makeShareButton(image, map));
        outer.appendChild(footer);
    } else {
        outer.appendChild(description);
    }

    return outer;
}

function makeShareButton(image, map) {
    const protocol = window.location.hostname === 'localhost' ? 'http' : 'https';
    const url = `${protocol}://${window.location.hostname}${window.location.pathname}?imageId=${image.id}&zoomLevel=${map.getZoom()}`;
    const icon = makeImageLink('#', 'static/share.png', 'share');

    icon.onclick = () => {
        navigator.share({
            title: `${document.title} #${image.id}`,
            text: 'Check out this cat!',
            url: url,
        })
            .then(() => console.log('catto sharing is catto caring'))
            .catch(error => console.log('error sharing:', error));
    }
    return icon;
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

function formatLocation(img) {
    const {city, country} = img;
    return city ? `${city}, ${country}` : country
}

function addCircle(img, map, radius) {
    const circle = L.circle([img.latitude, img.longitude], {color: 'red', radius: radius});
    const popup = circle.bindPopup(makePopupContent(img, map));
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

async function init(divId, accessToken) {
    map = L.map(divId);
    L.tileLayer(`https://api.mapbox.com/styles/v1/{id}/tiles/{z}/{x}/{y}?access_token=${accessToken}`, {
        maxZoom: maxZoomLevel,
        id: 'mapbox/streets-v11',
        tileSize: 512,
        zoomOffset: -1
    }).addTo(map);

    const response = await fetch('/images/');
    images = await response.json();

    const radius = calculateRadius(map.getZoom());
    images.forEach(img => img['circle'] = addCircle(img, map, radius));
    setMapView(map, images);

    map.on('zoomend', () => {
        updateCircleRadii(images, map.getZoom());
        updateStorage();
    });

    map.on('moveend', () => updateStorage());
    initPlaces(images, map);
}

// If url params with image id and optional zoom level are present, center map on that, otherwise try last location
// from local storage and fall back to default values (coords of first image).
function setMapView(map, images) {
    let zoomLevel = defaultZoomLevel;

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
    } else {
        let {latitude, longitude, zoomLevel} = readStorage(images[0].latitude, images[0].longitude)
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
            updateStorage();
        }

        li.appendChild(a);
        placesUl.appendChild(li);
    }
}

function readStorage(startLatitude, startLongitude) {
    const lsLat = localStorage.getItem('latitude')
    const lsLng = localStorage.getItem('longitude')
    const lsZoom = localStorage.getItem('zoomLevel');

    return {
        latitude: lsLat ? Number(lsLat) : startLatitude,
        longitude: lsLng ? Number(lsLng) : startLongitude,
        zoomLevel: lsZoom ? Number(lsZoom) : defaultZoomLevel,
    };
}

function updateStorage() {
    const {lat, lng} = map.getCenter();

    localStorage.setItem('latitude', lat);
    localStorage.setItem('longitude', lng);
    localStorage.setItem('zoomLevel', map.getZoom());
}
