const defaultZoomLevel = 15;
const defaultRadius = 12;
let [images, circles, map] = [[], [], null];

function makePopupContent(img) {
    const {id, timestamp} = img;
    const date = new Date(timestamp).toDateString();
    const location = formatLocation(img);

    return `<a href="images/${id}"><img src="images/${id}" alt="a photo of one or more cats"/></a>
        Taken on ${date} in ${location}`;
}

function formatLocation(img) {
    const {city, country} = img;
    return city ? `${city}, ${country}` : country
}

function addCircle(img, map) {
    const circle = L.circle([img.latitude, img.longitude], {color: 'red', radius: defaultRadius});
    circle.bindPopup(makePopupContent(img));
    circle.addTo(map);
    return circle;
}

function updateCircleRadii(circles, zoomLevel) {
    let radius = defaultRadius;
    if (zoomLevel >= 17) {
        radius -= (zoomLevel % 16) * 2;
    }
    radius = Math.max(radius, 1);

    circles.forEach(c => c.setRadius(radius));
}

function init(divId, accessToken, startLatitude, startLongitude) {
    map = L.map(divId).setView([startLatitude, startLongitude], defaultZoomLevel);

    L.tileLayer(`https://api.mapbox.com/styles/v1/{id}/tiles/{z}/{x}/{y}?access_token=${accessToken}`, {
        maxZoom: 22,
        id: 'mapbox/streets-v11',
        tileSize: 512,
        zoomOffset: -1
    }).addTo(map);

    fetch('/images/')
        .then(response => response.json()
            .then(d => {
                images = d
                circles = images.map(img => addCircle(img, map));
                map.on('zoomend', () => updateCircleRadii(circles, map.getZoom()));
                initPlaces(images, map);
            }));
}

function initPlaces(images, map) {
    const placesUl = document.getElementById('placesUl');
    const places = {};

    images.forEach(img => places[formatLocation(img)] = img);

    for (const label in places) {
        const li = document.createElement('li');
        const a = document.createElement('a');
        const {latitude, longitude} = places[label];

        console.log(`${label}: ${latitude} ${longitude}`);

        a.innerText = label;
        a.onclick = () => {
            document.getElementById('placesDropdown').removeAttribute('open');
            map.setView([latitude, longitude], defaultZoomLevel);
        }

        li.appendChild(a);
        placesUl.appendChild(li);
    }
}
